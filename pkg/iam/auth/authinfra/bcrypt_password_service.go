package authinfra

import (
	"github.com/Abraxas-365/ams/pkg/iam/user"
	"golang.org/x/crypto/bcrypt"
)

// BcryptPasswordService implementación del servicio de contraseñas usando bcrypt
type BcryptPasswordService struct {
	cost int
}

// NewBcryptPasswordService crea una nueva instancia del servicio de contraseñas
func NewBcryptPasswordService(cost int) user.PasswordService {
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	return &BcryptPasswordService{
		cost: cost,
	}
}

// HashPassword hashea una contraseña
func (s *BcryptPasswordService) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// VerifyPassword verifica una contraseña contra su hash
func (s *BcryptPasswordService) VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
