package ssas

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/CMSgov/bcda-app/bcda/database"
	"github.com/jinzhu/gorm"
	"log"
)

const DEFAULT_SCOPE = "bcda-api"

func InitializeSystemModels() *gorm.DB {
	log.Println("Initialize system models")
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	db.AutoMigrate(
		&System{},
		&EncryptionKey{},
	)

	db.Model(&System{}).AddForeignKey("group_id", "groups(group_id)", "RESTRICT", "RESTRICT")
	db.Model(&EncryptionKey{}).AddForeignKey("system_id", "systems(id)", "RESTRICT", "RESTRICT")

	return db
}

type System struct {
	gorm.Model
	GroupID        string          `json:"group_id"`
	ClientID       string          `json:"client_id"`
	SoftwareID     string          `json:"software_id"`
	ClientName     string          `json:"client_name"`
	ClientURI      string          `json:"client_uri"`
	APIScope	   string		   `json:"api_scope"`
	EncryptionKeys []EncryptionKey `json:"encryption_keys"`
}

type EncryptionKey struct {
	gorm.Model
	Body     string `json:"body"`
	System   System `gorm:"foreignkey:SystemID;association_foreignkey:ID"`
	SystemID uint   `json:"system_id"`
}

// RevokeSystemKeyPair soft deletes the active encryption key
// for the specified system so that it can no longer be used
func (system *System) RevokeSystemKeyPair() error {
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	var encryptionKey EncryptionKey

	err := db.Where("system_id = ?", system.ID).Find(&encryptionKey).Error
	if err != nil {
		return err
	}

	err = db.Delete(&encryptionKey).Error
	if err != nil {
		return err
	}

	return nil
}

/*
 GenerateSystemKeyPair creates a keypair for a system. The public key is saved to the database and the private key is returned.
*/
func (system *System) GenerateSystemKeyPair() (string, error) {
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	var key EncryptionKey
	if !db.Where("system_id = ?", system.ID).Find(&key).RecordNotFound() {
		return "", fmt.Errorf("encryption keypair already exists for system ID %d", system.ID)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("could not create key for system ID %d: %s", system.ID, err.Error())
	}

	publicKeyPKIX, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("could not marshal public key for system ID %d: %s", system.ID, err.Error())
	}

	publicKeyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyPKIX,
	})

	encryptionKey := EncryptionKey{
		Body:     string(publicKeyBytes),
		SystemID: system.ID,
	}

	err = db.Create(&encryptionKey).Error
	if err != nil {
		return "", fmt.Errorf("could not save key for system ID %d: %s", system.ID, err.Error())
	}

	privateKeyBytes := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		},
	)

	return string(privateKeyBytes), nil
}