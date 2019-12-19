package testUtils

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/CMSgov/bcda-app/bcda/client"
	"github.com/stretchr/testify/mock"
)

type BlueButtonClient struct {
	mock.Mock
	client.BlueButtonClient
	HICN *string
}

func (bbc *BlueButtonClient) GetExplanationOfBenefit(patientID, jobID, cmsID string) (string, error) {
	args := bbc.Called(patientID)
	return args.String(0), args.Error(1)
}

func (bbc *BlueButtonClient) GetPatientByIdentifierHash(hashedIdentifier string) (string, error) {
	args := bbc.Called(hashedIdentifier)
	return args.String(0), args.Error(1)
}

func (bbc *BlueButtonClient) GetPatient(patientID, jobID, cmsID string) (string, error) {
	args := bbc.Called(patientID, jobID, cmsID)
	return args.String(0), args.Error(1)
}

func (bbc *BlueButtonClient) GetCoverage(beneficiaryID, jobID, cmsID string) (string, error) {
	args := bbc.Called(beneficiaryID, jobID, cmsID)
	return args.String(0), args.Error(1)
}

// Returns copy of a static json file (From Blue Button Sandbox originally) after replacing the patient ID of 20000000000001 with the requested identifier
// This is private in the real function and should remain so, but in the test client it makes maintenance easier to expose it.
func (bbc *BlueButtonClient) GetData(endpoint, patientID string) (string, error) {
	var fData []byte
	fData, err := ioutil.ReadFile(filepath.Join("../shared_files/synthetic_beneficiary_data/", filepath.Clean(endpoint)))
	if err != nil {
		fData, err = ioutil.ReadFile(filepath.Join("../../shared_files/synthetic_beneficiary_data/", filepath.Clean(endpoint)))
		if err != nil {
			return "", err
		}
	}
	cleanData := strings.Replace(string(fData), "20000000000001", patientID, -1)
	if bbc.HICN != nil {
		cleanData = strings.Replace(cleanData, "c945694486322cfcf792a4d1c69ea88bf0c134edaf9f1d68a18466f9dc6ec2fe", client.HashIdentifier(*bbc.HICN), -1)
	}

	return cleanData, err
}
