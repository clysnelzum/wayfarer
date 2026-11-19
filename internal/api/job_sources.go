package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/clysnelzum/wayfarer/internal/linkedin"
)

const sourceRefTTL = 10 * time.Minute
const maxSourceRefBytes = 4096

var sourceRefPurpose = []byte("wayfarer/job-source/v1")

type sourceRefs struct{ aead cipher.AEAD }

type sourceRefEnvelope struct {
	Expires int64                     `json:"exp"`
	Link    linkedin.JobSourceContext `json:"link"`
}

func newSourceRefs() *sourceRefs {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil
	}
	return &sourceRefs{aead: aead}
}

func (refs *sourceRefs) seal(link *linkedin.JobSourceContext, now time.Time) string {
	if refs == nil || link == nil {
		return ""
	}
	body, err := json.Marshal(sourceRefEnvelope{Expires: now.Add(sourceRefTTL).Unix(), Link: *link})
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

func (refs *sourceRefs) open(ref, source string, now time.Time) *linkedin.JobSourceContext {
	if refs == nil || len(ref) > maxSourceRefBytes {
		return nil
	}
	sealed, err := base64.RawURLEncoding.Strict().DecodeString(ref)
	if err != nil || len(sealed) < refs.aead.NonceSize()+refs.aead.Overhead() {
		return nil
	}
	n := refs.aead.NonceSize()
	body, err := refs.aead.Open(nil, sealed[:n], sealed[n:], sourceRefPurpose)
	if err != nil {
		return nil
	}
	var envelope sourceRefEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Expires <= now.Unix() || envelope.Link.Source != source {
		return nil
	}
	return &envelope.Link
}
