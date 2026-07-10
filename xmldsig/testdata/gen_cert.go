//go:build ignore

// gen_cert.go gera testdata/test-cert.pfx — um PKCS#12 RSA autoassinado,
// exclusivo para testes, com chave FIXA embutida para tornar a assinatura
// (DigestValue/SignatureValue) determinística. NÃO usar em produção.
//
//	cd backend/pkg/xmldsig/testdata && GOWORK=off go run gen_cert.go
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"
)

const senha = "senha"

const chavePEM = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC8ZH/+PuSaROXv
hheOTddGiiEcYqoyD8lhgh1Cvd6kMi+MRQ6+3Dxpnq3V6AyAZyfIaItJps9YL3tD
s60Lg8gC8xRplhN4i7kyohqfOxOBQTHgAFEKYBsnYqRZhkbJNKwfI0UDEZaNlG6E
kFtD9oEVcPCWYlhxNomlyfhHE2dVuFEEKY5Yvb2QpssQ2rYLPkMz4VC21O0WJIiZ
DEppNRemVqSP0qG+G4fBSt6qaOwGpvyCHjxdcshu66HHMVkwM7X8VNDpnr6iK/EU
shngzRpQUL7Tll8bXJQAr8eS40HPoLTHoi0WZhAWXBmBV9Ol8S1zzSyYCh4r+34i
v6iBYuWbAgMBAAECggEASdtopmdHvgc20eGTCJIRzLDIbFVt/fRcceLNz+WaDGs0
YtyL/F1hPdMcuZiglhJa8WGzAavo69ypiZA+Th5a4nUj0oUomwDEGUqd/0Ds06aY
hAX7v4KQAq/UWNigla8Vr5tnKd3SVS0U6tmhPhK85ogBeiOSIshLzHhS4qKDx4Gu
9RHsAFwhWl8h5LE3LkbkTs2eZ9q+552UgQT778yXBWYKEw02ON/eyFhlZpoWCHFw
o6adaMRJoLYEKx8OUGjZvMGG2UHnm72KYDcsX8byWcT8amU5wut5ChS0Allnc/en
MEBC1x4wyUfv9JFxJ1RLka9kmK10pFo6uiEmXsuXEQKBgQDz5ilvOL/wk/zIfETp
h/fIsRLRR1v0GwN3amOvb9xETi6OLbl8dxMA4ASlo6J5d+07mjlk+Yv+HB9ncsf5
At9Yl15cFmqIAR2lj/CghYej9IzDMxXMeJLf92J6Po0p/LzJQVbJseL2ZwGPJDvr
AHg7NmxAgdtIbqNgbxX+8xquowKBgQDFvVU2NVKV6XqcEIil3kYk2GBivrQFPikq
n7P8Bguq69wCUz1Fiha6jNRMld7Xr1KMMkf4fHcfPrROU2IT7e2gZ6Z7GCp4Wpmt
b3ANzsdMpJFVxpFKXfIWQ5KmNL82kYG8Mo3gNJX+HtJNlFgppH3UW8dOtSo1YlN9
9WKdjMm0qQKBgHlTgD8Ukt6BL1koADvPaGJMO5khj0uB+Jp5sPb/hSnxXlVZx5Bz
x19DEOy41sTn9HoWwQBK5unUYYbbYUCsvh/axJS4jevz6iSG0uN3KT9VEoSzr99U
B3QkIre+0AgX8m2G9D5HICkEmpDzLF89j5GdBAKe1k9Uwi7hc6aslhRTAoGBALuT
3Hac5KfTIP1U7duy3AVtwRlK4Iisq+6EKv+jWIQeU8lc3x3NXbw/hdDUxt0BsqPN
utosb/AS0oYOsVZIeQeV66aPiRI8l9WfhcODaSn6XufogsDyF4kpNr82AEGqjFBU
AKQ0Hik/Ya44T93I7Zh6K7otP0n+6UbudUsRV/JpAoGBANpZGBWEL6iGX2jfNBXU
wnnJgJYDqc53SAR4yDfp3V3rI4Bauy2hObqGz2mcm5vSzBCDdGRnzzfkyW6r5Cz2
/IDD/rWpCot1B6v3nb5NO875lW/31OT6j7RuAw15MBusu13H0F9AeG59AIllwA25
IdLL3+Kmyia4Muv5R6Abj2Rn
-----END PRIVATE KEY-----`

func main() {
	block, _ := pem.Decode([]byte(chavePEM))
	if block == nil {
		panic("PEM inválido")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}
	rsaKey := key.(*rsa.PrivateKey)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(20250803),
		Subject: pkix.Name{
			CommonName:   "DEELP TESTE A1:00000000000191",
			Organization: []string{"DEELP TESTE"},
			Country:      []string{"BR"},
		},
		NotBefore:             time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &rsaKey.PublicKey, rsaKey)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}

	pfx, err := pkcs12.Modern.Encode(rsaKey, cert, nil, senha)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile("test-cert.pfx", pfx, 0o644); err != nil {
		panic(err)
	}
}
