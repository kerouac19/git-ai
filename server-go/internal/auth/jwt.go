package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Org struct {
	OrgID   string `json:"org_id"`
	OrgName string `json:"org_name"`
	OrgSlug string `json:"org_slug"`
	Role    string `json:"role"`
}

type TokenSubject struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	PersonalOrgID string `json:"personal_org_id"`
	Orgs          []Org  `json:"orgs"`
	Role          string `json:"role"`
}

type Claims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	Name          string `json:"name"`
	PersonalOrgID string `json:"personal_org_id"`
	Orgs          []Org  `json:"orgs"`
	Role          string `json:"role"`
	Type          string `json:"type"`
}

func SignAccessToken(subject TokenSubject, secret string, ttl time.Duration) (string, error) {
	return signToken(subject, secret, ttl, "access")
}

func SignRefreshToken(subject TokenSubject, secret string, ttl time.Duration) (string, error) {
	return signToken(subject, secret, ttl, "refresh")
}

func signToken(subject TokenSubject, secret string, ttl time.Duration, tokenType string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject.Sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Email:         subject.Email,
		Name:          subject.Name,
		PersonalOrgID: subject.PersonalOrgID,
		Orgs:          subject.Orgs,
		Role:          subject.Role,
		Type:          tokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func VerifyToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}
