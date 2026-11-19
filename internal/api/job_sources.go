package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

const sourceRefTTL = 10 * time.Minute
const maxSourceRefBytes = 4096

var sourceRefPurpose = []byte("wayfarer/job-source/v1")

type sourceRefs struct {
	aead cipher.AEAD
	now  func() time.Time
}

type sourceRefEnvelope struct {
	Expires       int64  `json:"exp"`
	Source        string `json:"source"`
	SourceContext string `json:"ctx"`
}

func newSourceRefs(now func() time.Time) (*sourceRefs, error) {
	if now == nil {
		now = time.Now
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, fmt.Errorf("generate source ref key: %w", err)
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create source ref cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create source ref AEAD: %w", err)
	}
	return &sourceRefs{aead: aead, now: now}, nil
}

func (refs *sourceRefs) seal(source, sourceContext string) string {
	if refs == nil || sourceContext == "" {
		return ""
	}
	now := refs.now()
	body, err := json.Marshal(sourceRefEnvelope{
		Expires:       now.Add(sourceRefTTL).Unix(),
		Source:        source,
		SourceContext: sourceContext,
	})
	if err != nil {
		return ""
	}
	nonce := make([]byte, refs.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ""
	}
	sealed := refs.aead.Seal(nonce, nonce, body, sourceRefPurpose)
	if base64.RawURLEncoding.EncodedLen(len(sealed)) > maxSourceRefBytes {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(sealed)
}

func (refs *sourceRefs) open(ref, source string) (string, bool) {
	if refs == nil || ref == "" || len(ref) > maxSourceRefBytes {
		return "", false
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(ref)
	if err != nil || len(sealed) < refs.aead.NonceSize()+refs.aead.Overhead() {
		return "", false
	}
	n := refs.aead.NonceSize()
	body, err := refs.aead.Open(nil, sealed[:n], sealed[n:], sourceRefPurpose)
	if err != nil {
		return "", false
	}
	var envelope sourceRefEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil ||
		envelope.Expires <= refs.now().Unix() ||
		envelope.Source != source ||
		envelope.SourceContext == "" {
		return "", false
	}
	return envelope.SourceContext, true
}
