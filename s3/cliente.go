// Package s3 fornece um cliente AWS S3 baseado em AWS SDK v2.
//
// Cobre os casos de uso comuns nos servicos Deelp:
//   - upload de bytes com metadados/MIME
//   - download como []byte ou io.ReadCloser (streaming)
//   - exclusao
//   - presigned URLs para GET
//   - listagem por prefixo (com paginacao automatica)
//   - HeadBucket para healthcheck
//   - configuracao opcional de CORS no bucket
//
// Importante: este pacote NAO le os.Getenv internamente. Toda config entra
// via struct Config — quem cria o cliente decide de onde vem o secret.
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Config struct {
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string

	// CORSOrigin, se nao vazio, aplica uma politica CORS simples ao bucket
	// no NewCliente (idempotente). Util quando o front-end consome
	// presigned URLs diretamente.
	CORSOrigin string

	Logger *slog.Logger
}

type Cliente struct {
	cli       *s3.Client
	presigner *s3.PresignClient
	bucket    string
	region    string
	logger    *slog.Logger
}

func (c Config) validar() error {
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("s3: Region obrigatório")
	}
	if strings.TrimSpace(c.Bucket) == "" {
		return errors.New("s3: Bucket obrigatório")
	}
	return nil
}

// NewCliente cria um novo cliente S3. Se CORSOrigin estiver definido, aplica
// a politica ao bucket de forma idempotente.
func NewCliente(cfg Config) (*Cliente, error) {
	if err := cfg.validar(); err != nil {
		return nil, err
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3: carregar config AWS: %w", err)
	}

	cli := s3.NewFromConfig(awsCfg)
	c := &Cliente{
		cli:       cli,
		presigner: s3.NewPresignClient(cli),
		bucket:    cfg.Bucket,
		region:    cfg.Region,
		logger:    cfg.Logger,
	}

	if cfg.CORSOrigin != "" {
		if err := c.configurarCORS(context.Background(), cfg.CORSOrigin); err != nil {
			if c.logger != nil {
				c.logger.Warn("s3: configurar CORS", "erro", err)
			}
		}
	}
	return c, nil
}

func (c *Cliente) configurarCORS(ctx context.Context, origem string) error {
	_, err := c.cli.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(c.bucket),
		CORSConfiguration: &types.CORSConfiguration{
			CORSRules: []types.CORSRule{{
				AllowedHeaders: []string{"*"},
				AllowedMethods: []string{"GET"},
				AllowedOrigins: []string{origem},
				ExposeHeaders:  []string{},
				MaxAgeSeconds:  aws.Int32(3600),
			}},
		},
	})
	return err
}

// Bucket devolve o nome do bucket configurado.
func (c *Cliente) Bucket() string { return c.bucket }

// Region devolve a regiao configurada.
func (c *Cliente) Region() string { return c.region }

// Raw expoe o *s3.Client da SDK v2 para casos que o pacote nao cobre.
func (c *Cliente) Raw() *s3.Client { return c.cli }

// Upload sobe bytes para o S3 com metadados opcionais. Sobrescreve se a
// chave ja existir.
func (c *Cliente) Upload(ctx context.Context, key string, conteudo []byte, mimeType string, metadados map[string]string) error {
	_, err := c.cli.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(conteudo),
		ContentType: aws.String(mimeType),
		ACL:         types.ObjectCannedACLPrivate,
		Metadata:    metadados,
	})
	if err != nil {
		return fmt.Errorf("s3: upload %q: %w", key, err)
	}
	return nil
}

// UploadCifrado sobe bytes com Server-Side Encryption AES256 ativado.
// Use para certificados A1/A3 e qualquer outro material sensivel onde o
// SSE deva proteger o conteudo em repouso. ContentType default
// "application/octet-stream" se vazio.
func (c *Cliente) UploadCifrado(ctx context.Context, key string, conteudo []byte, mimeType string, metadados map[string]string) error {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}
	_, err := c.cli.PutObject(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(c.bucket),
		Key:                  aws.String(key),
		Body:                 bytes.NewReader(conteudo),
		ContentType:          aws.String(mimeType),
		ACL:                  types.ObjectCannedACLPrivate,
		Metadata:             metadados,
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return fmt.Errorf("s3: upload cifrado %q: %w", key, err)
	}
	return nil
}

// Download le todo o conteudo da chave para []byte. Cuidado com objetos
// grandes — use LerStream nesses casos.
func (c *Cliente) Download(ctx context.Context, key string) ([]byte, error) {
	out, err := c.cli.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: download %q: %w", key, err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}

// LerStream devolve o io.ReadCloser direto — util para arquivos grandes
// (XML/ZIP) que nao cabem confortavelmente em memoria.
func (c *Cliente) LerStream(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := c.cli.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: get object %q: %w", key, err)
	}
	return out.Body, nil
}

// Excluir remove um objeto.
func (c *Cliente) Excluir(ctx context.Context, key string) error {
	_, err := c.cli.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3: delete %q: %w", key, err)
	}
	return nil
}

// PresignedURL gera URL temporaria para GET. Devolve a URL como string.
func (c *Cliente) PresignedURL(ctx context.Context, key string, validade time.Duration) (string, error) {
	out, err := c.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(validade))
	if err != nil {
		return "", fmt.Errorf("s3: presign %q: %w", key, err)
	}
	return out.URL, nil
}

// HeadBucket verifica acessibilidade do bucket. Util em healthchecks.
func (c *Cliente) HeadBucket(ctx context.Context) error {
	_, err := c.cli.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucket)})
	if err != nil {
		return fmt.Errorf("s3: head bucket %q: %w", c.bucket, err)
	}
	return nil
}

// ListarPrefixo lista chaves sob um prefixo. Lida com paginacao automaticamente.
func (c *Cliente) ListarPrefixo(ctx context.Context, prefixo string) ([]string, error) {
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(c.cli, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefixo),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3: list %q: %w", prefixo, err)
		}
		for _, item := range page.Contents {
			keys = append(keys, aws.ToString(item.Key))
		}
	}
	return keys, nil
}
