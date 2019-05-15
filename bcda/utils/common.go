package utils

import (
	"fmt"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
)

// FromEnv always returns a string that is either a non-empty value from the environment variable named by key or
// the string otherwise
func FromEnv(key, otherwise string) string {
	s := os.Getenv(key)
	if s == "" {
		logrus.Infof(`No %s value; using %s instead.`, key, otherwise)
		return otherwise
	}
	return s
}

func GetEnvInt(varName string, defaultVal int) int {
	v := os.Getenv(varName)
	if v != "" {
		i, err := strconv.Atoi(v)
		if err == nil {
			return i
		}
	}
	return defaultVal
}

// Look for a directory by increasingly looking up the directory tree by appending '.../'
// It will look a max of 5 levels up before accepting failure and returning an empty string and an error
func GetDirPath(dir string) (string, error) {

	for i := 0; i <= 5; i++ {
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		} else {
			// look one more level up
			dir = "../" + dir
		}
	}
	return "", fmt.Errorf("unable to locate %s in file path", dir)
}
