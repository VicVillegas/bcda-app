package models

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CMSgov/bcda-app/bcda/constants"

	authclient "github.com/CMSgov/bcda-app/bcda/auth/client"
	"github.com/CMSgov/bcda-app/bcda/auth/rsautils"
	"github.com/CMSgov/bcda-app/bcda/client"
	"github.com/CMSgov/bcda-app/bcda/database"
	"github.com/CMSgov/bcda-app/bcda/utils"
	"github.com/bgentry/que-go"
	"github.com/jinzhu/gorm"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const BCDA_FHIR_MAX_RECORDS_EOB_DEFAULT = 200
const BCDA_FHIR_MAX_RECORDS_PATIENT_DEFAULT = 5000
const BCDA_FHIR_MAX_RECORDS_COVERAGE_DEFAULT = 4000

func InitializeGormModels() *gorm.DB {
	log.Println("Initialize bcda models")
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	// Migrate the schema
	// Add your new models here
	// This should probably not be called in production
	// What happens when you need to make a database change, there's already data you need to preserve, and
	// you need to run a script to migrate existing data to its new home or shape?
	db.AutoMigrate(
		&ACO{},
		&User{},
		&Job{},
		&JobKey{},
		&CCLFBeneficiaryXref{},
		&CCLFFile{},
		&CCLFBeneficiary{},
		&Suppression{},
		&SuppressionFile{},
	)

	db.Model(&CCLFBeneficiary{}).AddForeignKey("file_id", "cclf_files(id)", "RESTRICT", "RESTRICT")

	return db
}

type Job struct {
	gorm.Model
	ACO               ACO       `gorm:"foreignkey:ACOID;association_foreignkey:UUID"` // aco
	ACOID             uuid.UUID `gorm:"type:char(36)" json:"aco_id"`
	User              User      `gorm:"foreignkey:UserID;association_foreignkey:UUID"` // user
	UserID            uuid.UUID `gorm:"type:char(36)"`
	RequestURL        string    `json:"request_url"` // request_url
	Status            string    `json:"status"`      // status
	JobCount          int
	CompletedJobCount int
	JobKeys           []JobKey
}

func (job *Job) CheckCompletedAndCleanup() (bool, error) {

	// Trivial case, no need to keep going
	if job.Status == "Completed" {
		return true, nil
	}
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	var completedJobs int64
	db.Model(&JobKey{}).Where("job_id = ?", job.ID).Count(&completedJobs)

	if int(completedJobs) >= job.JobCount {

		staging := fmt.Sprintf("%s/%d", os.Getenv("FHIR_STAGING_DIR"), job.ID)
		payload := fmt.Sprintf("%s/%d", os.Getenv("FHIR_PAYLOAD_DIR"), job.ID)

		files, err := ioutil.ReadDir(staging)
		if err != nil {
			log.Error(err)
		}

		for _, f := range files {
			oldPath := fmt.Sprintf("%s/%s",staging,f.Name())
			newPath := fmt.Sprintf("%s/%s",payload,f.Name())
			err := os.Rename(oldPath,newPath)
			if err != nil {
				log.Error(err)
			}
		}
		err = os.Remove(staging)
		if err != nil {
			log.Error(err)
		}
		return true, db.Model(&job).Update("status", "Completed").Error
	}

	return false, nil
}

func (job *Job) GetEnqueJobs(t string) (enqueJobs []*que.Job, err error) {
	maxBeneficiaries, err := GetMaxBeneCount(t)
	if err != nil {
		return nil, err
	}

	db := database.GetGORMDbConnection()
	defer database.Close(db)
	var aco ACO
	err = db.Find(&aco, "uuid = ?", job.ACOID).Error
	if err != nil {
		return nil, err
	}

	// includeSuppressed = false to exclude beneficiaries who have opted out of data sharing
	_, beneficiaries, err := aco.GetBeneficiaries(false)
	if err != nil {
		return nil, err
	}

	var chunkedBeneficiaries [][] string
	for i := 0; i < len(beneficiaries); i += maxBeneficiaries {
		end := i + maxBeneficiaries
		if end > len(beneficiaries) {
			end = len(beneficiaries)
		}
		chunkedBeneficiaries = append(chunkedBeneficiaries, beneficiaries[i:end])
	}
	
	for _, b_groups := range chunkedBeneficiaries {
		args, err := json.Marshal(jobEnqueueArgs{
			ID:             int(job.ID),
			ACOID:          job.ACOID.String(),
			UserID:         job.UserID.String(),
			BeneficiaryIDs: b_groups,
			ResourceType:   t,
		})
		if err != nil {
			return nil, err
		}
		j := &que.Job{
			Type: "ProcessJob",
			Args: args,
		}

		enqueJobs = append(enqueJobs, j)
	}

	return enqueJobs, nil
}

func (j *Job) StatusMessage() string {
	if j.Status == "In Progress" && j.JobCount > 0 {
		pct := float64(j.CompletedJobCount) / float64(j.JobCount) * 100
		return fmt.Sprintf("%s (%d%%)", j.Status, int(pct))
	}

	return j.Status
}

func GetMaxBeneCount(requestType string) (int, error) {
	var envVar string
	var defaultVal int

	switch requestType {
	case "ExplanationOfBenefit":
		envVar = "BCDA_FHIR_MAX_RECORDS_EOB"
		defaultVal = BCDA_FHIR_MAX_RECORDS_EOB_DEFAULT
	case "Patient":
		envVar = "BCDA_FHIR_MAX_RECORDS_PATIENT"
		defaultVal = BCDA_FHIR_MAX_RECORDS_PATIENT_DEFAULT
	case "Coverage":
		envVar = "BCDA_FHIR_MAX_RECORDS_COVERAGE"
		defaultVal = BCDA_FHIR_MAX_RECORDS_COVERAGE_DEFAULT
	default:
		err := errors.New("invalid request type")
		return -1, err
	}
	maxBeneficiaries := utils.GetEnvInt(envVar, defaultVal)

	return maxBeneficiaries, nil
}

type JobKey struct {
	gorm.Model
	Job          Job  `gorm:"foreignkey:jobID"`
	JobID        uint `gorm:"primary_key" json:"job_id"`
	EncryptedKey []byte
	FileName     string `gorm:"type:char(127)"`
	ResourceType string
}

// ACO represents an Accountable Care Organization.
type ACO struct {
	gorm.Model
	UUID        uuid.UUID `gorm:"primary_key;type:char(36)" json:"uuid"`
	CMSID       *string   `gorm:"type:char(5);unique" json:"cms_id"`
	Name        string    `json:"name"`
	ClientID    string    `json:"client_id"`
	GroupID     string    `json:"group_id"`
	SystemID    string    `json:"system_id"`
	AlphaSecret string    `json:"alpha_secret"`
	PublicKey   string    `json:"public_key"`
}

// GetBeneficiaries retrieves beneficiaries associated with the ACO.
// Two arrays are returned, for flexibility in function usage:
// One array contains the full CCLFBeneficiary model, fully populated
// The other array contains only the beneficiaries' IDs (in string format)
func (aco *ACO) GetBeneficiaries(includeSuppressed bool) ([]CCLFBeneficiary, []string, error) {
	var cclfBeneficiaries []CCLFBeneficiary
	var cclfBeneficiaryIds []string

	if aco.CMSID == nil {
		log.Errorf("No CMSID set for ACO: %s", aco.UUID)
		return cclfBeneficiaries, cclfBeneficiaryIds, fmt.Errorf("no CMS ID set for this ACO")
	}
	db := database.GetGORMDbConnection()
	defer database.Close(db)
	var cclfFile CCLFFile
	// todo add a filter here to make sure the file is up to date.
	if db.Where("aco_cms_id = ? and cclf_num = 8 and import_status= ?", aco.CMSID, constants.ImportComplete).Order("timestamp desc").First(&cclfFile).RecordNotFound() {
		log.Errorf("Unable to find CCLF8 File for ACO: %v", *aco.CMSID)
		return cclfBeneficiaries, cclfBeneficiaryIds, fmt.Errorf("unable to find cclfFile")
	}

	var suppressedHICNs []string
	if !includeSuppressed {
		db.Raw(`SELECT DISTINCT s.hicn
			FROM (
				SELECT hicn, MAX(effective_date) max_date
				FROM suppressions 
				WHERE effective_date <= NOW() AND preference_indicator != '' 
				GROUP BY hicn
			) h
			JOIN suppressions s ON s.hicn = h.hicn and s.effective_date = h.max_date
			WHERE preference_indicator = 'N'`).Pluck("hicn", &suppressedHICNs)
	}

	var err error
	if suppressedHICNs != nil {
		err = db.Not("hicn", suppressedHICNs).Find(&cclfBeneficiaries, "file_id = ?", cclfFile.ID).Error
		if err == nil {
			err = db.Table("cclf_beneficiaries").Not("hicn", suppressedHICNs).Select("id").Where("file_id = ?", cclfFile.ID).Pluck("id", &cclfBeneficiaryIds).Error
		}
	} else {
		err = db.Find(&cclfBeneficiaries, "file_id = ?", cclfFile.ID).Error
		if err == nil {
			err = db.Table("cclf_beneficiaries").Select("id").Where("file_id = ?", cclfFile.ID).Pluck("id", &cclfBeneficiaryIds).Error
		}
	}

	if err != nil {
		log.Errorf("Error retrieving beneficiaries from latest CCLF8 file for ACO ID %s: %s", aco.UUID.String(), err.Error())
		return nil, nil, err
	} else if len(cclfBeneficiaries) == 0 {
		log.Errorf("Found 0 beneficiaries from latest CCLF8 file for ACO ID %s", aco.UUID.String())
		return nil, nil, fmt.Errorf("found 0 beneficiaries from latest CCLF8 file for ACO ID %s", aco.UUID.String())
	} else if len(cclfBeneficiaryIds) == 0 {
                log.Errorf("Found 0 beneficiary IDs from latest CCLF8 file for ACO ID %s", aco.UUID.String())
                return nil, nil, fmt.Errorf("found 0 beneficiary IDs from latest CCLF8 file for ACO ID %s", aco.UUID.String())
        } else if len(cclfBeneficiaries) != len(cclfBeneficiaryIds) {
                log.Errorf("Beneficiary records do not match beneficiary ID records from latest CCLF8 file for ACO ID %s", aco.UUID.String())
                return nil, nil, fmt.Errorf("beneficiary records do not match beneficiary ID records from latest CCLF8 file for ACO ID %s", aco.UUID.String())
        }
	

	return cclfBeneficiaries, cclfBeneficiaryIds, nil
}

type CCLFBeneficiaryXref struct {
	gorm.Model
	FileID        uint   `gorm:"not null"`
	XrefIndicator string `json:"xref_indicator"`
	CurrentNum    string `json:"current_number"`
	PrevNum       string `json:"previous_number"`
	PrevsEfctDt   string `json:"effective_date"`
	PrevsObsltDt  string `json:"obsolete_date"`
}

// GetPublicKey returns the ACO's public key.
func (aco *ACO) GetPublicKey() (*rsa.PublicKey, error) {
	var key string
	if strings.ToLower(os.Getenv("BCDA_AUTH_PROVIDER")) == "ssas" {
		ssas, err := authclient.NewSSASClient()
		if err != nil {
			return nil, errors.Wrap(err, "cannot retrieve public key for ACO "+aco.UUID.String())
		}

		systemID, err := strconv.Atoi(aco.ClientID)
		if err != nil {
			return nil, errors.Wrap(err, "cannot retrieve public key for ACO "+aco.UUID.String())
		}

		keyBytes, err := ssas.GetPublicKey(systemID)
		if err != nil {
			return nil, errors.Wrap(err, "cannot retrieve public key for ACO "+aco.UUID.String())
		}

		key = string(keyBytes)
	} else {
		key = aco.PublicKey
	}
	return rsautils.ReadPublicKey(key)
}

func (aco *ACO) SavePublicKey(publicKey io.Reader) error {
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	k, err := ioutil.ReadAll(publicKey)
	if err != nil {
		return errors.Wrap(err, "cannot read public key for ACO "+aco.UUID.String())
	}

	key, err := rsautils.ReadPublicKey(string(k))
	if err != nil || key == nil {
		return errors.Wrap(err, "invalid public key for ACO "+aco.UUID.String())
	}

	aco.PublicKey = string(k)
	err = db.Save(&aco).Error
	if err != nil {
		return errors.Wrap(err, "cannot save public key for ACO "+aco.UUID.String())
	}

	return nil
}

// This exists to provide a known static keys used for ACO's in our alpha tests.
// This key is not meant to protect anything and both halves will be made available publicly
func GetATOPublicKey() *rsa.PublicKey {
	fmt.Println("Looking for a key at:")
	fmt.Println(os.Getenv("ATO_PUBLIC_KEY_FILE"))
	atoPublicKeyFile, err := os.Open(os.Getenv("ATO_PUBLIC_KEY_FILE"))
	if err != nil {
		fmt.Println("failed to open file")
		panic(err)
	}
	return utils.OpenPublicKeyFile(atoPublicKeyFile)
}

func GetATOPrivateKey() *rsa.PrivateKey {
	atoPrivateKeyFile, err := os.Open(os.Getenv("ATO_PRIVATE_KEY_FILE"))
	if err != nil {
		panic(err)
	}
	return utils.OpenPrivateKeyFile(atoPrivateKeyFile)
}

// CreateACO creates an ACO with the provided name and CMS ID.
func CreateACO(name string, cmsID *string) (uuid.UUID, error) {
	db := database.GetGORMDbConnection()
	defer database.Close(db)

	id := uuid.NewRandom()

	// TODO: remove ClientID below when a future refactor removes the need
	//    for every ACO to have a client_id at creation
	aco := ACO{Name: name, CMSID: cmsID, UUID: id, ClientID: id.String()}
	db.Create(&aco)

	return aco.UUID, db.Error
}

type User struct {
	gorm.Model
	UUID  uuid.UUID `gorm:"primary_key; type:char(36)" json:"uuid"` // uuid
	Name  string    `json:"name"`                                   // name
	Email string    `json:"email"`                                  // email
	ACO   ACO       `gorm:"foreignkey:ACOID;association_foreignkey:UUID"`
	ACOID uuid.UUID `gorm:"type:char(36)" json:"aco_id"` // aco_id
}

func CreateUser(name string, email string, acoUUID uuid.UUID) (User, error) {
	db := database.GetGORMDbConnection()
	defer database.Close(db)
	var aco ACO
	var user User
	// If we don't find the ACO return a blank user and an error
	if db.First(&aco, "UUID = ?", acoUUID).RecordNotFound() {
		return user, fmt.Errorf("unable to locate ACO with id of %v", acoUUID)
	}
	// check for duplicate email addresses and only make one if it isn't found
	if db.First(&user, "email = ?", email).RecordNotFound() {
		user = User{UUID: uuid.NewRandom(), Name: name, Email: email, ACOID: aco.UUID}
		db.Create(&user)
		return user, nil
	} else {
		return user, fmt.Errorf("unable to create user for %v because a user with that Email address already exists", email)
	}
}

// CLI command only support; note that we are choosing to fail quickly and let the user (one of us) figure it out
func CreateAlphaACO(acoCMSID string, db *gorm.DB) (ACO, error) {
	var count int
	db.Table("acos").Count(&count)
	aco := ACO{Name: fmt.Sprintf("Alpha ACO %d", count), UUID: uuid.NewRandom(), CMSID: &acoCMSID}
	db.Create(&aco)

	return aco, db.Error
}

type CCLFFile struct {
	gorm.Model
	CCLFNum         int       `gorm:"not null"`
	Name            string    `gorm:"not null;unique"`
	ACOCMSID        string    `gorm:"column:aco_cms_id"`
	Timestamp       time.Time `gorm:"not null"`
	PerformanceYear int       `gorm:"not null"`
	ImportStatus    string    `gorm:"column:import_status"`
}

func (cclfFile *CCLFFile) Delete() error {
	db := database.GetGORMDbConnection()
	defer db.Close()
	err := db.Unscoped().Where("file_id = ?", cclfFile.ID).Delete(&CCLFBeneficiary{}).Error
	if err != nil {
		return err
	}
	return db.Unscoped().Delete(&cclfFile).Error
}

// "The MBI has 11 characters, like the Health Insurance Claim Number (HICN), which can have up to 11."
// https://www.cms.gov/Medicare/New-Medicare-Card/Understanding-the-MBI-with-Format.pdf
type CCLFBeneficiary struct {
	gorm.Model
	CCLFFile     CCLFFile
	FileID       uint   `gorm:"not null;index:idx_cclf_beneficiaries_file_id"`
	HICN         string `gorm:"type:varchar(11);not null;index:idx_cclf_beneficiaries_hicn"`
	MBI          string `gorm:"type:char(11);not null;index:idx_cclf_beneficiaries_mbi"`
	BlueButtonID string `gorm:"type: text"`
}

type SuppressionFile struct {
	gorm.Model
	Name         string    `gorm:"not null;unique"`
	Timestamp    time.Time `gorm:"not null"`
	ImportStatus string    `gorm:"column:import_status"`
}

func (suppressionFile *SuppressionFile) Delete() error {
	db := database.GetGORMDbConnection()
	defer db.Close()
	err := db.Unscoped().Where("file_id = ?", suppressionFile.ID).Delete(&Suppression{}).Error
	if err != nil {
		return err
	}
	return db.Unscoped().Delete(&suppressionFile).Error
}

type Suppression struct {
	gorm.Model
	SuppressionFile     SuppressionFile
	FileID              uint      `gorm:"not null"`
	HICN                string    `gorm:"type:varchar(11);not null"`
	SourceCode          string    `gorm:"type:varchar(5)"`
	EffectiveDt         time.Time `gorm:"column:effective_date"`
	PrefIndicator       string    `gorm:"column:preference_indicator;type:char(1)"`
	SAMHSASourceCode    string    `gorm:"type:varchar(5)"`
	SAMHSAEffectiveDt   time.Time `gorm:"column:samhsa_effective_date"`
	SAMHSAPrefIndicator string    `gorm:"column:samhsa_preference_indicator;type:char(1)"`
	ACOCMSID            string    `gorm:"column:aco_cms_id;type:char(5)"`
	BeneficiaryLinkKey  int
}

// This method will ensure that a valid BlueButton ID is returned.
// If you use cclfBeneficiary.BlueButtonID you will not be guaranteed a valid value
func (cclfBeneficiary *CCLFBeneficiary) GetBlueButtonID(bb client.APIClient) (blueButtonID string, err error) {
	// If this is set already, just give it back.
	if cclfBeneficiary.BlueButtonID != "" {
		return cclfBeneficiary.BlueButtonID, nil
	}

	// didn't find a local value, need to ask BlueButton
	hashedHICN := client.HashHICN(cclfBeneficiary.HICN)
	jsonData, err := bb.GetPatientByHICNHash(hashedHICN)
	if err != nil {
		return "", err
	}
	var patient Patient
	err = json.Unmarshal([]byte(jsonData), &patient)
	if err != nil {
		log.Error(err)
		return "", err
	}

	if len(patient.Entry) == 0 {
		err = fmt.Errorf("patient identifier not found at Blue Button for CCLF beneficiary ID: %v", cclfBeneficiary.ID)
		log.Error(err)
		return "", err
	}
	var foundHICN = false
	var foundBlueButtonID = false
	blueButtonID = patient.Entry[0].Resource.ID
	for _, identifier := range patient.Entry[0].Resource.Identifier {
		if strings.Contains(identifier.System, "hicn-hash") {
			if identifier.Value == hashedHICN {
				foundHICN = true
			}
		} else if strings.Contains(identifier.System, "bene_id") {
			if identifier.Value == blueButtonID {
				foundBlueButtonID = true
			}
		}

	}
	if !foundHICN {
		err = fmt.Errorf("hashed HICN not found in the identifiers")
		log.Error(err)
		return "", err
	}
	if !foundBlueButtonID {
		err = fmt.Errorf("Blue Button identifier not found in the identifiers")
		log.Error(err)
		return "", err
	}

	return blueButtonID, nil
}

// This is not a persistent model so it is not necessary to include in GORM auto migrate.
// swagger:ignore
type jobEnqueueArgs struct {
	ID             int
	ACOID          string
	UserID         string
	BeneficiaryIDs []string
	ResourceType   string
}
