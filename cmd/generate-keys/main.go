// Command generate-keys creates one persistent RSA key pair for local setup.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"olixops-permission-center/pkg/logger"
)

func main() {
	if initial, err := logger.New("production"); err == nil {
		logger.Replace(initial)
	}
	if err := run(os.Args[1:]); err != nil {
		logger.L().Error("key generation failed", logger.ErrorField(err))
		_ = logger.Sync()
		os.Exit(1)
	}
	logger.L().Info("RSA key pair created; keep the private file secret and configure the service to reuse these files")
	_ = logger.Sync()
}

func run(args []string) error {
	flags := flag.NewFlagSet("generate-keys", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	private := flags.String("private-file", "", "New private PEM file in an existing directory")
	public := flags.String("public-file", "", "New public PEM file in an existing directory")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: generate-keys --private-file <new-private.pem> --public-file <new-public.pem>")
	}
	if strings.TrimSpace(*private) == "" || strings.TrimSpace(*public) == "" {
		return errors.New("--private-file and --public-file are required")
	}
	privatePath, err := newOutputPath(*private)
	if err != nil {
		return err
	}
	publicPath, err := newOutputPath(*public)
	if err != nil {
		return err
	}
	if privatePath == publicPath || (runtime.GOOS == "windows" && strings.EqualFold(privatePath, publicPath)) {
		return errors.New("private and public output files must be different")
	}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return errors.New("cannot generate RSA key pair")
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return errors.New("cannot encode private key")
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return errors.New("cannot encode public key")
	}
	return writeKeyPair(privatePath, publicPath,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
}

func newOutputPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("invalid output file path")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", errors.New("output parent directory must already exist")
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", errors.New("output parent directory must already exist")
	}
	resolved := filepath.Join(parent, filepath.Base(absolute))
	if _, err := os.Lstat(resolved); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("output file already exists or cannot be checked; no files were overwritten")
	}
	return resolved, nil
}

func writeKeyPair(privatePath, publicPath string, privatePEM, publicPEM []byte) error {
	private, err := os.OpenFile(privatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create private output file; no existing files were overwritten")
	}
	privateInfo, _ := private.Stat()
	committed := false
	defer func() {
		_ = private.Close()
		if !committed {
			removeCreatedFile(privatePath, privateInfo)
		}
	}()
	public, err := os.OpenFile(publicPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return errors.New("cannot create public output file; no existing files were overwritten")
	}
	publicInfo, _ := public.Stat()
	defer func() {
		_ = public.Close()
		if !committed {
			removeCreatedFile(publicPath, publicInfo)
		}
	}()
	if err := writeAndClose(private, privatePEM); err != nil {
		return errors.New("cannot save complete key pair")
	}
	if err := writeAndClose(public, publicPEM); err != nil {
		return errors.New("cannot save complete key pair")
	}
	committed = true
	return nil
}

func writeAndClose(file *os.File, contents []byte) error {
	if _, err := file.Write(contents); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return file.Close()
}

// Remove only a file created by this invocation, never a replacement at its path.
func removeCreatedFile(path string, created os.FileInfo) {
	if created == nil {
		return
	}
	if current, err := os.Lstat(path); err == nil && os.SameFile(created, current) {
		_ = os.Remove(path)
	}
}
