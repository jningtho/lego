package secure

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"

	"gopkg.in/square/go-jose.v2"
)

type JWS struct {
	privKey crypto.PrivateKey
	kid     string // Key identifier
	nonces  *NonceManager
}

func NewJWS(privKey crypto.PrivateKey, kid string, nonceManager *NonceManager) *JWS {
	return &JWS{
		privKey: privKey,
		nonces:  nonceManager,
		kid:     kid,
	}
}

func (j *JWS) SetKid(kid string) {
	j.kid = kid
}

func (j *JWS) SignContent(url string, content []byte) (*jose.JSONWebSignature, error) {
	var alg jose.SignatureAlgorithm
	switch k := j.privKey.(type) {
	case *rsa.PrivateKey:
		alg = jose.RS256
	case *ecdsa.PrivateKey:
		if k.Curve == elliptic.P256() {
			alg = jose.ES256
		} else if k.Curve == elliptic.P384() {
			alg = jose.ES384
		}
	}

	signKey := jose.SigningKey{
		Algorithm: alg,
		Key:       jose.JSONWebKey{Key: j.privKey, KeyID: j.kid},
	}

	options := jose.SignerOptions{
		NonceSource: j.nonces,
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": url,
		},
	}

	if j.kid == "" {
		options.EmbedJWK = true
	}

	signer, err := jose.NewSigner(signKey, &options)
	if err != nil {
		return nil, fmt.Errorf("failed to create jose signer -> %v", err)
	}

	signed, err := signer.Sign(content)
	if err != nil {
		return nil, fmt.Errorf("failed to sign content -> %v", err)
	}
	return signed, nil
}

func (j *JWS) SignEABContent(url, kid string, hmac []byte) (*jose.JSONWebSignature, error) {
	jwk := jose.JSONWebKey{Key: j.privKey}
	jwkJSON, err := jwk.Public().MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("acme: error encoding eab jwk key: %v", err)
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: hmac},
		&jose.SignerOptions{
			EmbedJWK: false,
			ExtraHeaders: map[jose.HeaderKey]interface{}{
				"kid": kid,
				"url": url,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create External Account Binding jose signer -> %v", err)
	}

	signed, err := signer.Sign(jwkJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to External Account Binding sign content -> %v", err)
	}

	return signed, nil
}

func (j *JWS) GetKeyAuthorization(token string) (string, error) {
	var publicKey crypto.PublicKey
	switch k := j.privKey.(type) {
	case *ecdsa.PrivateKey:
		publicKey = k.Public()
	case *rsa.PrivateKey:
		publicKey = k.Public()
	}

	// Generate the Key Authorization for the challenge
	jwk := &jose.JSONWebKey{Key: publicKey}
	if jwk == nil {
		return "", errors.New("could not generate JWK from key")
	}

	thumbBytes, err := jwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return "", err
	}

	// unpad the base64URL
	keyThumb := base64.RawURLEncoding.EncodeToString(thumbBytes)

	return token + "." + keyThumb, nil
}