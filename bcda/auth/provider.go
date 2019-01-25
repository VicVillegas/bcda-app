package auth

import "github.com/dgrijalva/jwt-go"

// Provider is an interface for operations performed through an authentication provider.
type Provider interface {
	RegisterClient(params []byte) ([]byte, error)
	UpdateClient(params []byte) ([]byte, error)
	DeleteClient(params []byte) error

	GenerateClientCredentials(params []byte) ([]byte, error)
	RevokeClientCredentials(params []byte) error

	RequestAccessToken(params []byte) (Token, error)
	RevokeAccessToken(tokenString string) error

	ValidateJWT(tokenString string) error
	DecodeJWT(tokenString string) (jwt.Token, error)
}
