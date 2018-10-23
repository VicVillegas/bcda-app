package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/CMSgov/bcda-app/bcda/database"
	"github.com/CMSgov/bcda-app/bcda/models"
	"github.com/CMSgov/bcda-app/bcda/testUtils"
	"github.com/bgentry/que-go"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	"os"
	"testing"
)

var eobData = `{
	"resourceType": "Bundle",
	"id": "c6a3f338-5f25-4001-9b8f-023d9297c6ce",
	"meta": {
	  "lastUpdated": "2018-10-01T17:02:22.544-04:00"
	},
	"type": "searchset",
	"total": 1,
	"link": [],
	"entry": []
}`

type MockBlueButtonClient struct {
	mock.Mock
}

type MockBlueButtonClientWithError struct {
	mock.Mock
}

type MainTestSuite struct {
	testUtils.AuthTestSuite
}

func (s *MainTestSuite) SetupTest() {
	os.Setenv("FHIR_PAYLOAD_DIR", "data/test")
	os.Setenv("BB_CLIENT_CERT_FILE", "../shared_files/bb-dev-test-cert.pem")
	os.Setenv("BB_CLIENT_KEY_FILE", "../shared_files/bb-dev-test-key.pem")
	os.Setenv("BB_CLIENT_CA_FILE", "../shared_files/test-server-cert.pem")
	models.InitializeGormModels()
}

func (s *MainTestSuite) TearDownTest() {
	testUtils.PrintSeparator()
}

func TestMainTestSuite(t *testing.T) {
	suite.Run(t, new(MainTestSuite))
}

func TestWriteEOBDataToFile(t *testing.T) {
	os.Setenv("FHIR_PAYLOAD_DIR", "data/test")
	bbc := MockBlueButtonClient{}
	acoID := "9c05c1f8-349d-400f-9b69-7963f2262b07"
	beneficiaryIDs := []string{"10000", "11000"}

	err := writeEOBDataToFile(&bbc, acoID, beneficiaryIDs)
	if err != nil {
		t.Fail()
	}

	filePath := fmt.Sprintf("%s/%s.ndjson", os.Getenv("FHIR_PAYLOAD_DIR"), acoID)
	fData, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fail()
	}

	assert.Equal(t, eobData+"\n"+eobData+"\n", string(fData))

	os.Remove(filePath)
}

func TestWriteEOBDataToFileNoClient(t *testing.T) {
	err := writeEOBDataToFile(nil, "9c05c1f8-349d-400f-9b69-7963f2262b08", []string{"20000", "21000"})
	assert.NotNil(t, err)
}

func TestWriteEOBDataToFileInvalidACO(t *testing.T) {
	bbc := MockBlueButtonClient{}
	acoID := "9c05c1f8-349d-400f-9b69-7963f2262zzz"
	beneficiaryIDs := []string{"10000", "11000"}

	err := writeEOBDataToFile(&bbc, acoID, beneficiaryIDs)
	assert.NotNil(t, err)
}

func TestWriteEOBDataToFileWithError(t *testing.T) {
	os.Setenv("FHIR_PAYLOAD_DIR", "data/test")
	bbc := MockBlueButtonClientWithError{}
	acoID := "9c05c1f8-349d-400f-9b69-7963f2262b07"
	beneficiaryIDs := []string{"10000", "11000"}

	err := writeEOBDataToFile(&bbc, acoID, beneficiaryIDs)
	if err != nil {
		t.Fail()
	}

	filePath := fmt.Sprintf("%s/%s-error.ndjson", os.Getenv("FHIR_PAYLOAD_DIR"), acoID)
	fData, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fail()
	}

	ooResp := `{"resourceType":"OperationOutcome","issue":[{"severity":"Error","code":"Exception","details":{"coding":[{"display":"Error retrieving ExplanationOfBenefit for beneficiary 10000 in ACO 9c05c1f8-349d-400f-9b69-7963f2262b07"}],"text":"Error retrieving ExplanationOfBenefit for beneficiary 10000 in ACO 9c05c1f8-349d-400f-9b69-7963f2262b07"}}]}
{"resourceType":"OperationOutcome","issue":[{"severity":"Error","code":"Exception","details":{"coding":[{"display":"Error retrieving ExplanationOfBenefit for beneficiary 11000 in ACO 9c05c1f8-349d-400f-9b69-7963f2262b07"}],"text":"Error retrieving ExplanationOfBenefit for beneficiary 11000 in ACO 9c05c1f8-349d-400f-9b69-7963f2262b07"}}]}`
	assert.Equal(t, ooResp+"\n", string(fData))

	os.Remove(fmt.Sprintf("%s/%s.ndjson", os.Getenv("FHIR_PAYLOAD_DIR"), acoID))
	os.Remove(filePath)
}

func TestAppendErrorToFile(t *testing.T) {
	os.Setenv("FHIR_PAYLOAD_DIR", "data/test")
	acoID := "9c05c1f8-349d-400f-9b69-7963f2262b07"

	appendErrorToFile(acoID, "", "", "")

	filePath := fmt.Sprintf("%s/%s-error.ndjson", os.Getenv("FHIR_PAYLOAD_DIR"), acoID)
	fData, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fail()
	}

	ooResp := `{"resourceType":"OperationOutcome","issue":[{"severity":"Error"}]}`

	assert.Equal(t, ooResp+"\n", string(fData))

	os.Remove(filePath)
}

func (bbc *MockBlueButtonClient) GetExplanationOfBenefitData(patientID string) (string, error) {
	return eobData, nil
}

func (bbc *MockBlueButtonClientWithError) GetExplanationOfBenefitData(patientID string) (string, error) {
	return "", errors.New("Error")
}

func (s *MainTestSuite) TestProcessJob() {
	db := database.GetGORMDbConnection()
	defer db.Close()

	j := models.Job{
		AcoID:      uuid.Parse("DBBD1CE1-AE24-435C-807D-ED45953077D3"),
		UserID:     uuid.Parse("82503A18-BF3B-436D-BA7B-BAE09B7FFD2F"),
		RequestURL: "/api/v1/Patient/$export",
		Status:     "Pending",
	}
	db.Save(&j)

	jobArgs := new(jobEnqueueArgs)
	jobArgs.ID = int(j.ID)
	jobArgs.AcoID = j.AcoID.String()
	jobArgs.UserID = j.UserID.String()
	jobArgs.BeneficiaryIDs = []string{"10000", "11000"}
	args, _ := json.Marshal(jobArgs)

	job := &que.Job{
		Type: "ProcessJob",
		Args: args,
	}
	fmt.Println("About to queue up the job")
	err := processJob(job)
	assert.Nil(s.T(), err)
	var completedJob models.Job
	err = db.First(&completedJob, "ID = ?", jobArgs.ID).Error
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "Completed", completedJob.Status)
}

func (s *MainTestSuite) TestSetupQueue() {
	setupQueue()
	os.Setenv("WORKER_POOL_SIZE", "7")
	setupQueue()
}
