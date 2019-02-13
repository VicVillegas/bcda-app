package auth

import (
	"os"
	"strings"

	"github.com/dgrijalva/jwt-go"
	log "github.com/sirupsen/logrus"
)

const (
	Alpha = "alpha"
	Okta  = "okta"
)

var providerName = Alpha

func init () {
	log.SetFormatter(&log.JSONFormatter{})
	SetProvider(strings.ToLower(os.Getenv(`BCDA_AUTH_PROVIDER`)))
}

func SetProvider(name string) {
	if name != "" {
		switch (strings.ToLower(name)) {
		case Okta:
			providerName = name
		case Alpha:
			providerName = name
		default:
			log.Infof(`Unknown providerName %s; using %s`, name, providerName)
		}
	}
	log.Infof(`Auth is made possible by %s`, providerName)
}

func GetProvider() Provider {
	switch providerName {
	case Alpha:
		return AlphaAuthPlugin{}
	case Okta:
		return OktaAuthPlugin{}
	default:
		return AlphaAuthPlugin{}
	}
}

type Credentials struct {
	ClientID     string
	ClientSecret string
	Token        Token
}

// Provider defines operations performed through an authentication provider.
type Provider interface {
	// Ask the auth Provider to register a software client for the ACO identified by localID.
	RegisterClient(localID string) (Credentials, error)

	// Update data associated with the registered software client identified by clientID
	UpdateClient(params []byte) ([]byte, error)

	// Delete the registered software client identified by clientID, revoking an active tokens
	DeleteClient(params []byte) error

	// Generate new or replace existing Credentials for the given clientID
	GenerateClientCredentials(params []byte) ([]byte, error)

	// Revoke any existing Credentials for the given clientID
	RevokeClientCredentials(params []byte) error

	// Request an access token with a specific time-to-live for the given clientID
	RequestAccessToken(params []byte) (Token, error)

	// Revoke a specific access token identified in a base64 encoded token string
	RevokeAccessToken(tokenString string) error

	// Assert that a base64 encoded token string is valid for accessing the BCDA API
	ValidateJWT(tokenString string) error

	// Decode a base64 encoded token string
	DecodeJWT(tokenString string) (jwt.Token, error)
}
