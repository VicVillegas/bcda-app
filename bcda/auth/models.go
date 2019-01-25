package auth

import (
	jwt "github.com/dgrijalva/jwt-go"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/pborman/uuid"

	"github.com/CMSgov/bcda-app/bcda/database"
	"github.com/CMSgov/bcda-app/bcda/models"
)

func InitializeGormModels() *gorm.DB {
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	// Migrate the schema
	// Add your new models here
	db.AutoMigrate(
		&Token{},
	)

	return db
}

type Token struct {
	gorm.Model
	// even though gorm.Model has an `id` field declared as the primary key, the following definition overrides that
	UUID        uuid.UUID   `gorm:"primary_key" json:"uuid"` // uuid (primary key)
	User        models.User `gorm:"foreignkey:UserID;association_foreignkey:UUID"`
	UserID      uuid.UUID   `json:"user_id"`                                      // user_id
	Value       string      `gorm:"type:varchar(511); unique" json:"value"`       // Deprecated: When can we drop Value without hurting existing alpha tokens?
	Active      bool        `json:"active"`                                       // active
	ACO         models.ACO  `gorm:"foreignkey:ACOID;association_foreignkey:UUID"` // ACO needed here because user can belong to multiple ACOs
	ACOID       uuid.UUID   `json:"aco_id"`                                       // aco_id
	IssuedAt    int64       `json:"issued_at"`                                    // standard token claim; unix date
	ExpiresOn   int64       `json:"expires_on"`                                   // standard token claim; unix date
	TokenString string      `gorm:"-"`                                            // ignore; not for database
}

// When getting a Token out of the database, reconstruct its string value and store it in TokenString.
func (t *Token) AfterFind() error {
	s, err := GenerateTokenString(t.UUID, t.UserID, t.ACOID, t.IssuedAt, t.ExpiresOn)
	if err == nil {
		t.TokenString = s
		return nil
	}
	return err
}

// Given all claim values, construct a token string.
func GenerateTokenString(id, userID, acoID uuid.UUID, issuedAt int64, expiresOn int64) (string, error) {
	token := jwt.New(jwt.SigningMethodRS512)
	token.Claims = jwt.MapClaims{
		"exp": expiresOn,
		"iat": issuedAt,
		"sub": userID.String(),
		"aco": acoID.String(),
		"id":  id.String(),
	}
	return token.SignedString(InitAuthBackend().PrivateKey)
}
