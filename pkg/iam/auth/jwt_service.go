package auth

import (
	"fmt"
	"time"

	"github.com/Abraxas-365/ams/pkg/config"
	"github.com/Abraxas-365/ams/pkg/kernel"
	"github.com/golang-jwt/jwt/v5"
)

// JWTService implementación del TokenService usando JWT
type JWTService struct {
	secretKey       []byte
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration
	issuer          string
	audience        []string
}

// NewJWTService crea una nueva instancia del servicio JWT
func NewJWTServiceFromConfig(cfg *config.JWTConfig) *JWTService {
	return &JWTService{
		secretKey:       []byte(cfg.SecretKey),
		accessTokenTTL:  cfg.AccessTokenTTL,
		refreshTokenTTL: cfg.RefreshTokenTTL,
		issuer:          cfg.Issuer,
		audience:        cfg.Audience,
	}
}

// Claims personalizados para JWT
type JWTClaims struct {
	UserID   kernel.UserID   `json:"user_id"`
	TenantID kernel.TenantID `json:"tenant_id"`
	Email    string          `json:"email"`
	Name     string          `json:"name"`
	Scopes   []string        `json:"scopes"`
	jwt.RegisteredClaims
}

// GenerateAccessToken genera un token de acceso JWT
func (j *JWTService) GenerateAccessToken(userID kernel.UserID, tenantID kernel.TenantID, claims map[string]any) (string, error) {
	now := time.Now()

	// Extraer claims adicionales
	email, _ := claims["email"].(string)
	name, _ := claims["name"].(string)
	scopes, _ := claims["scopes"].([]string)

	// Default to empty scopes if not provided
	if scopes == nil {
		scopes = []string{}
	}

	jwtClaims := JWTClaims{
		UserID:   userID,
		TenantID: tenantID,
		Email:    email,
		Name:     name,
		Scopes:   scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    j.issuer,
			Subject:   userID.String(),
			Audience:  j.audience,
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessTokenTTL)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)

	tokenString, err := token.SignedString(j.secretKey)
	if err != nil {
		return "", ErrTokenGenerationFailed().WithDetail("error", err.Error())
	}

	return tokenString, nil
}

// ValidateAccessToken valida y decodifica un token de acceso
func (j *JWTService) ValidateAccessToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		// Verificar el método de firma
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secretKey, nil
	})

	if err != nil {
		return nil, ErrTokenValidationFailed().WithDetail("error", err.Error())
	}

	if !token.Valid {
		return nil, ErrTokenValidationFailed().WithDetail("error", "token is invalid")
	}

	jwtClaims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, ErrTokenValidationFailed().WithDetail("error", "invalid claims type")
	}

	return &TokenClaims{
		UserID:    jwtClaims.UserID,
		TenantID:  jwtClaims.TenantID,
		Email:     jwtClaims.Email,
		Name:      jwtClaims.Name,
		Scopes:    jwtClaims.Scopes,
		IssuedAt:  jwtClaims.IssuedAt.Time,
		ExpiresAt: jwtClaims.ExpiresAt.Time,
	}, nil
}

// GenerateRefreshToken genera un token de refresh simple
func (j *JWTService) GenerateRefreshToken(userID kernel.UserID) (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		Issuer:    j.issuer,
		Subject:   userID.String(),
		Audience:  j.audience,
		ExpiresAt: jwt.NewNumericDate(now.Add(j.refreshTokenTTL)),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(j.secretKey)
	if err != nil {
		return "", ErrTokenGenerationFailed().WithDetail("error", err.Error())
	}

	return tokenString, nil
}
