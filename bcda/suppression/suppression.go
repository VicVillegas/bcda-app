package suppression

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/CMSgov/bcda-app/bcda/utils"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/CMSgov/bcda-app/bcda/database"
	"github.com/CMSgov/bcda-app/bcda/models"
)

type suppressionFileMetadata struct {
	name         string
	timestamp    time.Time
	filePath     string
	imported     bool
	deliveryDate time.Time
}

const (
	headerCode        = "HDR_BENEDATASHR"
	trailerCode       = "TRL_BENEDATASHR"
)

func ImportSuppressionDirectory(filePath string) (success, failure, skipped int, err error) {
	var suppresslist []suppressionFileMetadata

	err = filepath.Walk(filePath, getSuppressionFileMetadata(&suppresslist, &skipped))
	if err != nil {
		return 0, 0, 0, err
	}

	if len(suppresslist) == 0 {
		log.Info("Failed to find any suppression files in directory")
		return 0, 0, skipped, nil
	}

	for _, metadata := range suppresslist {
		err = validate(metadata)
		if err != nil {
			fmt.Printf("Failed to validate suppression file: %s.\n", metadata)
			log.Errorf("Failed to validate suppression file: %s", metadata)
			failure++
		} else {
			if err = importSuppressionData(metadata); err != nil {
				fmt.Printf("Failed to import suppression file: %s.\n", metadata)
				log.Errorf("Failed to import suppression file: %s ", metadata)
				failure++
			} else {
				metadata.imported = true
				success++
			}
		}
	}
	err = cleanupSuppression(suppresslist)
	if err != nil {
		log.Error(err)
	}

	if failure > 0 {
		err = errors.New("one or more suppression files failed to import correctly")
		log.Error(err)
	} else {
		err = nil
	}
	return success, failure, skipped, err
}

func getSuppressionFileMetadata(suppresslist *[]suppressionFileMetadata, skipped *int) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			var fileName = "nil"
			if info != nil {
				fileName = info.Name()
			}
			fmt.Printf("Error in checking suppression file %s: %s.\n", fileName, err)
			err = errors.Wrapf(err, "error in checking suppression file: %s,", fileName)
			log.Error(err)
			return err
		}
		// Directories are not Suppression files
		if info.IsDir() {
			return nil
		}

		metadata, err := parseMetadata(info.Name())
		metadata.filePath = path
		metadata.deliveryDate = info.ModTime()
		if err != nil {
			// skipping files with a bad name.  An unknown file in this dir isn't a blocker
			fmt.Printf("Unknown file found: %s.\n", metadata)
			log.Errorf("Unknown file found: %s", metadata)
			*skipped = *skipped + 1

			deleteThreshold := time.Hour * time.Duration(utils.GetEnvInt("ARCHIVE_THRESHOLD_HR", 24))
			if metadata.deliveryDate.Add(deleteThreshold).Before(time.Now()) {
				newpath := fmt.Sprintf("%s/%s", os.Getenv("PENDING_DELETION_DIR"), info.Name())
				err = os.Rename(metadata.filePath, newpath)
				if err != nil {
					fmt.Printf("Error moving unknown file %s to pending deletion dir.\n", metadata)
					err = fmt.Errorf("error moving unknown file %s to pending deletion dir", metadata)
					log.Error(err)
					return err
				}
			}
			return nil
		}
		*suppresslist = append(*suppresslist, metadata)
		return nil
	}
}

func parseMetadata(filename string) (suppressionFileMetadata, error) {
	var metadata suppressionFileMetadata
	// Beneficiary Data Sharing Preferences File sent by 1-800-Medicare: P#EFT.ON.ACO.NGD1800.DPRF.Dyymmdd.Thhmmsst
	// Prefix: T = test, P = prod;
	filenameRegexp := regexp.MustCompile(`((P|T)\#EFT)\.ON\.ACO\.NGD1800\.DPRF\.(D\d{6}\.T\d{6})\d`)
	filenameMatches := filenameRegexp.FindStringSubmatch(filename)
	if len(filenameMatches) < 4 {
		fmt.Printf("Invalid filename for file: %s.\n", filename)
		err := fmt.Errorf("invalid filename for file: %s", filename)
		log.Error(err)
		return metadata, err
	}

	filenameDate := filenameMatches[3]
	t, err := time.Parse("D060102.T150405", filenameDate)
	if err != nil || t.IsZero() {
		fmt.Printf("Failed to parse date '%s' from file: %s.\n", filenameDate, filename)
		err = errors.Wrapf(err, "failed to parse date '%s' from file: %s", filenameDate, filename)
		log.Error(err)
		return metadata, err
	}

	metadata.timestamp = t
	metadata.name = filenameMatches[0]

	return metadata, nil
}

func validate(metadata suppressionFileMetadata) error {
	fmt.Printf("Validating suppression file %s...\n", metadata)
	log.Infof("Validating suppression file %s...", metadata)

	f, err := os.Open(metadata.filePath)
	if err != nil {
		fmt.Printf("Could not read file %s.\n", metadata)
		err = errors.Wrapf(err, "could not read file %s", metadata)
		log.Error(err)
		return err
	}
	defer f.Close()

	var (
		headTrailStart, headTrailEnd = 0, 15
		recCountStart, recCountEnd   = 23, 33
	)

	sc := bufio.NewScanner(f)
	count := 0
	for sc.Scan() {
		b := sc.Bytes()
		metaInfo := string(bytes.TrimSpace(b[headTrailStart:headTrailEnd]))
		if count == 0 {
			if metaInfo != headerCode {
				// invalid file header found
				fmt.Printf("Invalid file header for file: %s.\n", metadata.filePath)
				err := fmt.Errorf("invalid file header for file: %s", metadata.filePath)
				log.Error(err)
				return err
			}
			count++
			continue
		}

		if metaInfo != trailerCode {
			count++
		} else {
			// trailer info
			expectedCount, err := strconv.Atoi(string(bytes.TrimSpace(b[recCountStart:recCountEnd])))
			if err != nil {
				fmt.Printf("Failed to parse record count from file: %s.\n", metadata.filePath)
				err = fmt.Errorf("failed to parse record count from file: %s", metadata.filePath)
				log.Error(err)
				return err
			}
			// subtract the single count from the header
			count--
			if count != expectedCount {
				fmt.Printf("Incorrect number of records found from file: '%s'. Expected record count: %d, Actual record count: %d.\n", metadata.filePath, expectedCount, count)
				err = fmt.Errorf("incorrect number of records found from file: '%s'. Expected record count: %d, Actual record count: %d", metadata.filePath, expectedCount, count)
				log.Error(err)
				return err
			}
		}
	}
	fmt.Printf("Successfully validated suppression file %s.\n", metadata)
	log.Infof("Successfully validated suppression file %s.", metadata)
	return nil
}

func importSuppressionData(metadata suppressionFileMetadata) error {
	err := importSuppressionMetadata(metadata, func(fileID uint, b []byte, db *gorm.DB) error {
		var (
			hicnStart, hicnEnd                           = 0, 11
			lKeyStart, lKeyEnd                           = 11, 21
			effectiveDtStart, effectiveDtEnd             = 354, 362
			sourceCdeStart, sourceCdeEnd                 = 362, 367
			prefIndtorStart, prefIndtorEnd               = 368, 369
			samhsaEffectiveDtStart, samhsaEffectiveDtEnd = 369, 377
			samhsaSourceCdeStart, samhsaSourceCdeEnd     = 377, 382
			samhsaPrefIndtorStart, samhsaPrefIndtorEnd   = 383, 384
			acoIdStart, acoIdEnd                         = 384, 389
		)
		ds := string(bytes.TrimSpace(b[effectiveDtStart:effectiveDtEnd]))
		dt, err := convertDt(ds)
		if err != nil {
			fmt.Printf("Failed to parse the effective date '%s' from file: %s.\n", ds, metadata.filePath)
			err = errors.Wrapf(err, "failed to parse the effective date '%s' from file: %s", ds, metadata.filePath)
			log.Error(err)
			return err
		}
		ds = string(bytes.TrimSpace(b[samhsaEffectiveDtStart:samhsaEffectiveDtEnd]))
		samhsaDt, err := convertDt(ds)
		if err != nil {
			fmt.Printf("Failed to parse the samhsa effective date '%s' from file: %s.\n", ds, metadata.filePath)
			err = errors.Wrapf(err, "failed to parse the samhsa effective date '%s' from file: %s", ds, metadata.filePath)
			log.Error(err)
			return err
		}
		keyval := string(bytes.TrimSpace(b[lKeyStart:lKeyEnd]))
		if keyval == "" {
			keyval = "0"
		}
		lk, err := strconv.Atoi(keyval)
		if err != nil {
			fmt.Printf("Failed to parse beneficiary link key from file: %s.\n", metadata.filePath)
			err = errors.Wrapf(err, "failed to parse beneficiary link key from file: %s", metadata.filePath)
			log.Error(err)
			return err
		}

		suppression := &models.Suppression{
			FileID:              fileID,
			HICN:                string(bytes.TrimSpace(b[hicnStart:hicnEnd])),
			SourceCode:          string(bytes.TrimSpace(b[sourceCdeStart:sourceCdeEnd])),
			EffectiveDt:         dt,
			PrefIndicator:       string(bytes.TrimSpace(b[prefIndtorStart:prefIndtorEnd])),
			SAMHSASourceCode:    string(bytes.TrimSpace(b[samhsaSourceCdeStart:samhsaSourceCdeEnd])),
			SAMHSAEffectiveDt:   samhsaDt,
			SAMHSAPrefIndicator: string(bytes.TrimSpace(b[samhsaPrefIndtorStart:samhsaPrefIndtorEnd])),
			BeneficiaryLinkKey:  lk,
			ACOCMSID:            string(bytes.TrimSpace(b[acoIdStart:acoIdEnd])),
		}
		err = db.Create(suppression).Error
		if err != nil {
			fmt.Println("Could not create suppression record.")
			err = errors.Wrap(err, "could not create suppression record")
			log.Error(err)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

func importSuppressionMetadata(metadata suppressionFileMetadata, importFunc func(uint, []byte, *gorm.DB) error) error {
	fmt.Printf("Importing suppression file %s...\n", metadata)
	log.Infof("Importing suppression file %s...", metadata)

	var (
		headTrailStart, headTrailEnd = 0, 15
	)

	suppressionMetaFile := &models.SuppressionFile{
		Name:      metadata.name,
		Timestamp: metadata.timestamp,
	}

	db := database.GetGORMDbConnection()
	defer database.Close(db)

	err := db.Create(&suppressionMetaFile).Error
	if err != nil {
		fmt.Printf("Could not create suppression file record for file: %s. \n", metadata)
		err = errors.Wrapf(err, "could not create suppression file record for file: %s.", metadata)
		log.Error(err)
		return err
	}

	importStatusInterval := utils.GetEnvInt("SUPPRESS_IMPORT_STATUS_RECORDS_INTERVAL", 1000)
	importedCount := 0
	f, err := os.Open(metadata.filePath)
	if err != nil {
		fmt.Printf("Could not read file %s.\n", metadata)
		err = errors.Wrapf(err, "could not read file %s", metadata)
		log.Error(err)
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		b := sc.Bytes()
		if len(bytes.TrimSpace(b)) > 0 {
			metaInfo := string(bytes.TrimSpace(b[headTrailStart:headTrailEnd]))
			if metaInfo == headerCode || metaInfo == trailerCode {
				continue
			}
			err = importFunc(suppressionMetaFile.ID, b, db)
			if err != nil {
				log.Error(err)
				return err
			}
			importedCount++
			if importedCount%importStatusInterval == 0 {
				fmt.Printf("Suppression records imported: %d\n", importedCount)
			}
		}
	}

	successMsg := fmt.Sprintf("Successfully imported %d records from suppression file %s.", importedCount, metadata)
	fmt.Println(successMsg)
	log.Infof(successMsg)

	return nil
}

func cleanupSuppression(suppresslist []suppressionFileMetadata) error {
	errCount := 0
	for _, suppressionFile := range suppresslist {
		fmt.Printf("Cleaning up file %s.\n", suppressionFile)
		log.Infof("Cleaning up file %s", suppressionFile)
		newpath := fmt.Sprintf("%s/%s", os.Getenv("PENDING_DELETION_DIR"), suppressionFile.name)
		if !suppressionFile.imported {
			// check the timestamp on the failed files
			elapsed := time.Since(suppressionFile.deliveryDate).Hours()
			deleteThreshold := utils.GetEnvInt("ARCHIVE_THRESHOLD_HR", 24)
			if int(elapsed) > deleteThreshold {
				err := os.Rename(suppressionFile.filePath, newpath)
				if err != nil {
					errCount++
					errMsg := fmt.Sprintf("File %s failed to clean up properly: %v", suppressionFile, err)
					fmt.Println(errMsg)
					log.Error(errMsg)
				} else {
					fmt.Printf("File %s never ingested, moved to the pending deletion dir.\n", suppressionFile)
					log.Infof("File %s never ingested, moved to the pending deletion dir", suppressionFile)
				}
			}
		} else {
			// move the successful files to the deletion dir
			err := os.Rename(suppressionFile.filePath, newpath)
			if err != nil {
				errCount++
				errMsg := fmt.Sprintf("File %s failed to clean up properly: %v", suppressionFile, err)
				fmt.Println(errMsg)
				log.Error(errMsg)
			} else {
				fmt.Printf("File %s successfully ingested, moved to the pending deletion dir.\n", suppressionFile)
				log.Infof("File %s successfully ingested, moved to the pending deletion dir", suppressionFile)
			}
		}
	}
	if errCount > 0 {
		return fmt.Errorf("%d files could not be cleaned up", errCount)
	}
	return nil
}

func convertDt(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse("20060102", s)
	if err != nil || t.IsZero() {
		return t, err
	}
	return t, nil
}

func (m suppressionFileMetadata) String() string {
	if m.filePath != "" {
		return m.filePath
	}
	return m.name
}
