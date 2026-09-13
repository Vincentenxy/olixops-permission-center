package credential

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const (
	vectorID     = "0123456789abcdef0123456789abcdef"
	vectorToken  = "opc_" + vectorID + ".AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	vectorDigest = "d152c55483852aee3c7c96fd21cca9d328300fc0c3caa8e69f555d3523677ff2"
)

func TestGenerateUniqueVerifiableKeys(t *testing.T) {
	ids, tokens := make(map[string]bool), make(map[string]bool)
	for range 32 {
		issued, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		if len(issued.ID) != 32 || len(issued.Token) != 80 || len(issued.Digest) != 64 {
			t.Fatal("unexpected credential format lengths")
		}
		id, err := ParseID(issued.Token)
		if err != nil || id != issued.ID || !Verify(issued.Token, issued.Digest) {
			t.Fatal("generated credential does not round-trip")
		}
		if ids[id] || tokens[issued.Token] {
			t.Fatal("random identifiers or secrets repeated")
		}
		ids[id], tokens[issued.Token] = true, true
		encoded, err := json.Marshal(issued)
		if err != nil {
			t.Fatal(err)
		}
		var data map[string]string
		if err := json.Unmarshal(encoded, &data); err != nil {
			t.Fatal(err)
		}
		if len(data) != 1 || data["id"] != issued.ID || strings.Contains(string(encoded), issued.Token) || strings.Contains(string(encoded), issued.Digest) {
			t.Fatal("JSON exposed credential material")
		}
	}
}

func TestKnownVectorAndTampering(t *testing.T) {
	id, err := ParseID(vectorToken)
	if err != nil || id != vectorID || !Verify(vectorToken, vectorDigest) {
		t.Fatal("canonical vector or SHA-256 digest rejected")
	}
	for _, token := range []string{"opc_f" + vectorToken[5:], vectorToken[:37] + "B" + vectorToken[38:]} {
		if _, err := ParseID(token); err != nil {
			t.Fatal("tampered vector must remain syntactically valid")
		}
		if Verify(token, vectorDigest) {
			t.Fatal("modified token matched original digest")
		}
	}
	for _, digest := range []string{"", vectorDigest[:63], vectorDigest + "0", strings.ToUpper(vectorDigest), "g" + vectorDigest[1:], "0" + vectorDigest[1:], " " + vectorDigest[1:]} {
		if Verify(vectorToken, digest) {
			t.Fatal("malformed or incorrect digest accepted")
		}
	}
}

func TestParseRejectsNoncanonicalTokens(t *testing.T) {
	for name, token := range map[string]string{
		"empty": "", "short": vectorToken[:79], "long": vectorToken + "A",
		"wrong prefix": "xyz_" + vectorToken[4:], "uppercase prefix": "OPC_" + vectorToken[4:],
		"uppercase ID":    "opc_" + strings.ToUpper(vectorID) + vectorToken[36:],
		"nonhex ID":       "opc_g" + vectorToken[5:],
		"wrong separator": vectorToken[:36] + "_" + vectorToken[37:],
		"padded base64":   vectorToken[:79] + "=", "standard base64": vectorToken[:37] + "+" + vectorToken[38:],
		"noncanonical trailing bits": vectorToken[:79] + "9",
		"secret newline":             vectorToken[:50] + "\n" + vectorToken[51:],
		"leading whitespace":         " " + vectorToken, "trailing whitespace": vectorToken + " ",
		"unicode": vectorToken[:37] + "密" + vectorToken[40:], "huge": strings.Repeat("fake-sensitive", 10000),
	} {
		t.Run(name, func(t *testing.T) {
			id, err := ParseID(token)
			if id != "" || !errors.Is(err, ErrInvalidToken) || err.Error() != "invalid API key" {
				t.Fatal("malformed credential did not produce safe generic error")
			}
			if Verify(token, vectorDigest) {
				t.Fatal("malformed token accepted")
			}
		})
	}
}

func TestGenerationConsumesIndependentRandomBytes(t *testing.T) {
	data := make([]byte, 48)
	for i := range data {
		data[i] = byte(i)
	}
	issued, err := generate(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if issued.ID != hex.EncodeToString(data[:16]) || issued.Token != "opc_"+issued.ID+"."+base64.RawURLEncoding.EncodeToString(data[16:]) {
		t.Fatal("ID and secret must use separate random bytes")
	}
	if !Verify(issued.Token, issued.Digest) {
		t.Fatal("digest does not cover the complete token")
	}
	failed, err := generate(bytes.NewReader(data[:47]))
	if !errors.Is(err, errGenerationFailed) || failed != (Issued{}) {
		t.Fatal("partial randomness produced a credential")
	}
	failed, err = generate(failedReader{})
	if !errors.Is(err, errGenerationFailed) || strings.Contains(err.Error(), "fake-sensitive") || failed != (Issued{}) {
		t.Fatal("randomness failure leaked details or returned partial credentials")
	}
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("fake-sensitive-random-source") }
