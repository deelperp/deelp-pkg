// Package s3 fornece um cliente S3 padronizado para todos os serviços
// que precisam armazenar arquivos (certificados A1/A3 do nfe-service /
// mdfe-service / cliente-service, anexos de OS no estoque-service,
// avatares no autenticacao-service, etc.).
//
// Diferenças da implementação anterior (estoque-service):
//   - NÃO chama os.Getenv internamente. Toda configuração entra via Config.
//     Quem cria o cliente decide de onde vem o secret (env, secret manager).
//   - Não loga para stdout direto; usa slog opcional.
//   - Upload/download genéricos não conhecem domínio (multipart fica fora);
//     wrappers específicos (avatar, certificado, anexo) podem ficar no serviço.
package s3

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type Config struct {
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string

	// CORSOrigin, se não vazio, aplica uma política CORS simples ao bucket
	// no NewCliente (idempotente). Útil quando o front-end consome
	// presigned URLs diretamente.
	CORSOrigin string

	Logger *slog.Logger
}

type Cliente struct {
	sess       *session.Session
	cli        *s3.S3
	uploader   *s3manager.Uploader
	downloader *s3manager.Downloader
	bucket     string
	region     string
	logger     *slog.Logger
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

// NewCliente cria um novo cliente S3 e, se CORSOrigin estiver definido,
// aplica a política ao bucket de forma idempotente.
func NewCliente(cfg Config) (*Cliente, error) {
	if err := cfg.validar(); err != nil {
		return nil, err
	}

	awsCfg := &aws.Config{Region: aws.String(cfg.Region)}
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		awsCfg.Credentials = credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, "")
	}

	sess, err := session.NewSession(awsCfg)
	if err != nil {
		return nil, fmt.Errorf("s3: sessão: %w", err)
	}

	c := &Cliente{
		sess:       sess,
		cli:        s3.New(sess),
		uploader:   s3manager.NewUploader(sess),
		downloader: s3manager.NewDownloader(sess),
		bucket:     cfg.Bucket,
		region:     cfg.Region,
		logger:     cfg.Logger,
	}

	if cfg.CORSOrigin != "" {
		if err := c.configurarCORS(cfg.CORSOrigin); err != nil {
			if c.logger != nil {
				c.logger.Warn("s3: configurar CORS", "erro", err)
			}
		}
	}
	return c, nil
}

func (c *Cliente) configurarCORS(origem string) error {
	maxAge := int64(3600)
	_, err := c.cli.PutBucketCors(&s3.PutBucketCorsInput{
		Bucket: aws.String(c.bucket),
		CORSConfiguration: &s3.CORSConfiguration{
			CORSRules: []*s3.CORSRule{{
				AllowedHeaders: aws.StringSlice([]string{"*"}),
				AllowedMethods: aws.StringSlice([]string{"GET"}),
				AllowedOrigins: aws.StringSlice([]string{origem}),
				ExposeHeaders:  aws.StringSlice([]string{}),
				MaxAgeSeconds:  &maxAge,
			}},
		},
	})
	return err
}

// Bucket devolve o nome do bucket configurado.
func (c *Cliente) Bucket() string { return c.bucket }

// Region devolve a região configurada.
func (c *Cliente) Region() string { return c.region }

// Upload sobe bytes para o S3 com metadados opcionais. Sobrescreve se a
// chave já existir.
func (c *Cliente) Upload(key string, conteudo []byte, mimeType string, metadados map[string]string) error {
	metaPtr := make(map[string]*string, len(metadados))
	for k, v := range metadados {
		v := v
		metaPtr[k] = &v
	}
	_, err := c.uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(conteudo),
		ContentType: aws.String(mimeType),
		ACL:         aws.String("private"),
		Metadata:    metaPtr,
	})
	if err != nil {
		return fmt.Errorf("s3: upload %q: %w", key, err)
	}
	return nil
}

// Download lê o conteúdo da chave.
func (c *Cliente) Download(key string) ([]byte, error) {
	buf := aws.NewWriteAtBuffer([]byte{})
	_, err := c.downloader.Download(buf, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: download %q: %w", key, err)
	}
	return buf.Bytes(), nil
}

// Excluir remove um objeto.
func (c *Cliente) Excluir(key string) error {
	_, err := c.cli.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3: delete %q: %w", key, err)
	}
	return nil
}

// PresignedURL gera URL temporária para GET.
func (c *Cliente) PresignedURL(key string, validade time.Duration) (string, error) {
	req, _ := c.cli.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	return req.Presign(validade)
}

// HeadBucket verifica acessibilidade do bucket. Útil em healthchecks.
func (c *Cliente) HeadBucket() error {
	_, err := c.cli.HeadBucket(&s3.HeadBucketInput{Bucket: aws.String(c.bucket)})
	if err != nil {
		return fmt.Errorf("s3: head bucket %q: %w", c.bucket, err)
	}
	return nil
}

// ListarPrefixo lista chaves sob um prefixo (paginação simples).
func (c *Cliente) ListarPrefixo(prefixo string) ([]string, error) {
	out, err := c.cli.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefixo),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: list %q: %w", prefixo, err)
	}
	keys := make([]string, 0, len(out.Contents))
	for _, item := range out.Contents {
		keys = append(keys, aws.StringValue(item.Key))
	}
	return keys, nil
}

// LerStream útil para casos que precisam do io.ReadCloser direto (CSV/XML
// grande não cabem confortavelmente em []byte).
func (c *Cliente) LerStream(key string) (io.ReadCloser, error) {
	out, err := c.cli.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: get object %q: %w", key, err)
	}
	return out.Body, nil
}
