package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateKeysCreatesMatchingPairAndRefusesOverwrite(t *testing.T) {
	directory := t.TempDir()
	privatePath, publicPath := filepath.Join(directory, "private.pem"), filepath.Join(directory, "public.pem")
	args := []string{"--private-file", privatePath, "--public-file", publicPath}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	privatePEM, err := os.ReadFile(privatePath)
	if err != nil {
		t.Fatal(err)
	}
	publicPEM, err := os.ReadFile(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	privateBlock, _ := pem.Decode(privatePEM)
	publicBlock, _ := pem.Decode(publicPEM)
	if privateBlock == nil || privateBlock.Type != "PRIVATE KEY" || publicBlock == nil || publicBlock.Type != "PUBLIC KEY" {
		t.Fatal("unexpected PEM format")
	}
	privateValue, err := x509.ParsePKCS8PrivateKey(privateBlock.Bytes)
	if err != nil {
		t.Fatal("invalid private key encoding")
	}
	publicValue, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		t.Fatal("invalid public key encoding")
	}
	key, ok := privateValue.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() != 3072 || key.Validate() != nil || !key.PublicKey.Equal(publicValue) {
		t.Fatal("generated keys do not match the required RSA policy")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(privatePath)
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatal("private key permissions expose it to other users")
		}
	}
	if err := run(args); err == nil {
		t.Fatal("existing pair was accepted")
	}
	assertFileContents(t, privatePath, privatePEM)
	assertFileContents(t, publicPath, publicPEM)
}

func TestGenerateKeysValidatesPathsWithoutCreatingFiles(t *testing.T) {
	for _, testCase := range []string{"missing flags", "missing public", "same path", "private exists", "public exists", "missing parent", "unknown flag"} {
		t.Run(testCase, func(t *testing.T) {
			directory := t.TempDir()
			privatePath, publicPath := filepath.Join(directory, "private.pem"), filepath.Join(directory, "public.pem")
			args := []string{"--private-file", privatePath, "--public-file", publicPath}
			switch testCase {
			case "missing flags":
				args = nil
			case "missing public":
				args = args[:2]
			case "same path":
				args[3] = privatePath
			case "private exists":
				if err := os.WriteFile(privatePath, []byte("existing private"), 0600); err != nil {
					t.Fatal(err)
				}
			case "public exists":
				if err := os.WriteFile(publicPath, []byte("existing public"), 0644); err != nil {
					t.Fatal(err)
				}
			case "missing parent":
				args[1] = filepath.Join(directory, "absent", "private.pem")
			case "unknown flag":
				args = append(args, "--unknown-sensitive-placeholder")
			}
			if err := run(args); err == nil {
				t.Fatal("invalid arguments accepted")
			}
			if testCase == "private exists" {
				assertFileContents(t, privatePath, []byte("existing private"))
			} else {
				assertAbsent(t, privatePath)
			}
			if testCase == "public exists" {
				assertFileContents(t, publicPath, []byte("existing public"))
			} else {
				assertAbsent(t, publicPath)
			}
		})
	}
}

func TestPublicCreationConflictDiscardsOnlyNewPrivateFile(t *testing.T) {
	directory := t.TempDir()
	privatePath, publicPath := filepath.Join(directory, "private.pem"), filepath.Join(directory, "public.pem")
	if err := os.WriteFile(publicPath, []byte("concurrently created public file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeKeyPair(privatePath, publicPath, []byte("new test private"), []byte("new test public")); err == nil {
		t.Fatal("creation race overwrote existing public file")
	}
	assertAbsent(t, privatePath)
	assertFileContents(t, publicPath, []byte("concurrently created public file"))
}

func assertFileContents(t *testing.T, path string, expected []byte) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != string(expected) {
		t.Fatal("existing output file was changed")
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("unexpected partial output file")
	}
}
