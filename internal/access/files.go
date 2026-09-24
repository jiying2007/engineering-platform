package access

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"

	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// Configuration paths and their parents must be host-controlled. No file bytes,
// credentials, or parser input are interpolated into configuration errors.
func ReadConfiguration(path string, secret bool) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open configuration: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || (secret && info.Mode().Perm()&0o077 != 0) {
		return nil, fmt.Errorf("unsafe configuration file permissions")
	}
	data, err := io.ReadAll(io.LimitReader(file, strictjson.MaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > strictjson.MaxBytes {
		return nil, fmt.Errorf("configuration unreadable or outside size limit")
	}
	return data, nil
}

func LoadServerTLS(certFile, keyFile, clientCAFile string) (*tls.Config, error) {
	certPEM, err := ReadConfiguration(certFile, false)
	if err != nil {
		return nil, err
	}
	keyPEM, err := ReadConfiguration(keyFile, true)
	if err != nil {
		return nil, err
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("invalid server certificate/key pair")
	}
	caPEM, err := ReadConfiguration(clientCAFile, false)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	count := 0
	for len(caPEM) > 0 {
		var block *pem.Block
		block, caPEM = pem.Decode(caPEM)
		if block == nil || block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("invalid client CA PEM")
		}
		ca, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !ca.IsCA || ca.KeyUsage&x509.KeyUsageCertSign == 0 {
			return nil, fmt.Errorf("invalid client CA certificate")
		}
		roots.AddCert(ca)
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("client CA is empty")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, Certificates: []tls.Certificate{cert}, SessionTicketsDisabled: true}, nil
}
