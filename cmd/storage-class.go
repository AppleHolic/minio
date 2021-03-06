/*
 * Minio Cloud Storage, (C) 2017 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	// metadata entry for storage class
	amzStorageClass = "x-amz-storage-class"
	// Canonical metadata entry for storage class
	amzStorageClassCanonical = "X-Amz-Storage-Class"
	// Reduced redundancy storage class
	reducedRedundancyStorageClass = "REDUCED_REDUNDANCY"
	// Standard storage class
	standardStorageClass = "STANDARD"
	// Reduced redundancy storage class environment variable
	reducedRedundancyStorageClassEnv = "MINIO_STORAGE_CLASS_RRS"
	// Standard storage class environment variable
	standardStorageClassEnv = "MINIO_STORAGE_CLASS_STANDARD"
	// Supported storage class scheme is EC
	supportedStorageClassScheme = "EC"
	// Minimum parity disks
	minimumParityDisks = 2
	defaultRRSParity   = 2
)

// Struct to hold storage class
type storageClass struct {
	Scheme string
	Parity int
}

type storageClassConfig struct {
	Standard storageClass `json:"standard"`
	RRS      storageClass `json:"rrs"`
}

// Validate if storage class in metadata
// Only Standard and RRS Storage classes are supported
func isValidStorageClassMeta(sc string) bool {
	return sc == reducedRedundancyStorageClass || sc == standardStorageClass
}

func (sc *storageClass) UnmarshalText(b []byte) error {
	scStr := string(b)
	if scStr != "" {
		s, err := parseStorageClass(scStr)
		if err != nil {
			return err
		}
		sc.Parity = s.Parity
		sc.Scheme = s.Scheme
	} else {
		sc = &storageClass{}
	}

	return nil
}

func (sc *storageClass) MarshalText() ([]byte, error) {
	if sc.Scheme != "" && sc.Parity != 0 {
		return []byte(fmt.Sprintf("%s:%d", sc.Scheme, sc.Parity)), nil
	}
	return []byte(""), nil
}

// Parses given storageClassEnv and returns a storageClass structure.
// Supported Storage Class format is "Scheme:Number of parity disks".
// Currently only supported scheme is "EC".
func parseStorageClass(storageClassEnv string) (sc storageClass, err error) {
	s := strings.Split(storageClassEnv, ":")

	// only two elements allowed in the string - "scheme" and "number of parity disks"
	if len(s) > 2 {
		return storageClass{}, errors.New("Too many sections in " + storageClassEnv)
	} else if len(s) < 2 {
		return storageClass{}, errors.New("Too few sections in " + storageClassEnv)
	}

	// only allowed scheme is "EC"
	if s[0] != supportedStorageClassScheme {
		return storageClass{}, errors.New("Unsupported scheme " + s[0] + ". Supported scheme is EC")
	}

	// Number of parity disks should be integer
	parityDisks, err := strconv.Atoi(s[1])
	if err != nil {
		return storageClass{}, err
	}

	sc = storageClass{
		Scheme: s[0],
		Parity: parityDisks,
	}

	return sc, nil
}

// Validates the parity disks for Reduced Redundancy storage class
func validateRRSParity(rrsParity, ssParity int) (err error) {
	disks := len(globalEndpoints)
	// disks < 4 means this is not a erasure coded setup and so storage class is not supported
	if disks < 4 {
		return fmt.Errorf("Setting storage class only allowed for erasure coding mode")
	}

	// Reduced redundancy storage class is not supported for 4 disks erasure coded setup.
	if disks == 4 && rrsParity != 0 {
		return fmt.Errorf("Reduced redundancy storage class not supported for " + strconv.Itoa(disks) + " disk setup")
	}

	// RRS parity disks should be greater than or equal to minimumParityDisks. Parity below minimumParityDisks is not recommended.
	if rrsParity < minimumParityDisks {
		return fmt.Errorf("Reduced redundancy storage class parity should be greater than or equal to " + strconv.Itoa(minimumParityDisks))
	}

	// Reduced redundancy implies lesser parity than standard storage class. So, RRS parity disks should be
	// - less than N/2, if StorageClass parity is not set.
	// - less than StorageClass Parity, if Storage class parity is set.
	switch ssParity {
	case 0:
		if rrsParity >= disks/2 {
			return fmt.Errorf("Reduced redundancy storage class parity disks should be less than " + strconv.Itoa(disks/2))
		}
	default:
		if rrsParity >= ssParity {
			return fmt.Errorf("Reduced redundancy storage class parity disks should be less than " + strconv.Itoa(ssParity))
		}
	}

	return nil
}

// Validates the parity disks for Standard storage class
func validateSSParity(ssParity, rrsParity int) (err error) {
	disks := len(globalEndpoints)
	// disks < 4 means this is not a erasure coded setup and so storage class is not supported
	if disks < 4 {
		return fmt.Errorf("Setting storage class only allowed for erasure coding mode")
	}

	// Standard storage class implies more parity than Reduced redundancy storage class. So, Standard storage parity disks should be
	// - greater than or equal to 2, if RRS parity is not set.
	// - greater than RRS Parity, if RRS parity is set.
	switch rrsParity {
	case 0:
		if ssParity < minimumParityDisks {
			return fmt.Errorf("Standard storage class parity disks should be greater than or equal to " + strconv.Itoa(minimumParityDisks))
		}
	default:
		if ssParity <= rrsParity {
			return fmt.Errorf("Standard storage class parity disks should be greater than " + strconv.Itoa(rrsParity))
		}
	}

	// Standard storage class parity should be less than or equal to N/2
	if ssParity > disks/2 {
		return fmt.Errorf("Standard storage class parity disks should be less than or equal to " + strconv.Itoa(disks/2))
	}

	return nil
}

// Returns the data and parity drive count based on storage class
// If storage class is set using the env vars MINIO_STORAGE_CLASS_RRS and MINIO_STORAGE_CLASS_STANDARD
// -- corresponding values are returned
// If storage class is not set using environment variables, default values are returned
// -- Default for Reduced Redundancy Storage class is, parity = 2 and data = N-Parity
// -- Default for Standard Storage class is, parity = N/2, data = N/2
// If storage class is not present in metadata, default value is data = N/2, parity = N/2
func getRedundancyCount(sc string, totalDisks int) (data, parity int) {
	parity = totalDisks / 2
	switch sc {
	case reducedRedundancyStorageClass:
		if globalRRStorageClass.Parity != 0 {
			// set the rrs parity if available
			parity = globalRRStorageClass.Parity
		} else {
			// else fall back to default value
			parity = defaultRRSParity
		}
	case standardStorageClass:
		if globalStandardStorageClass.Parity != 0 {
			// set the standard parity if available
			parity = globalStandardStorageClass.Parity
		}
	}
	// data is always totalDisks - parity
	return totalDisks - parity, parity
}

// Returns per object readQuorum and writeQuorum
// readQuorum is the minimum required disks to read data.
// writeQuorum is the minimum required disks to write data.
func objectQuorumFromMeta(xl xlObjects, partsMetaData []xlMetaV1, errs []error) (objectReadQuorum, objectWriteQuorum int, err error) {

	// get the latest updated Metadata and a count of all the latest updated xlMeta(s)
	latestXLMeta, count := getLatestXLMeta(partsMetaData, errs)

	// latestXLMeta is updated most recently.
	// We implicitly assume that all the xlMeta(s) have same dataBlocks and parityBlocks.
	// We now check that at least dataBlocks number of xlMeta is available. This means count
	// should be greater than or equal to dataBlocks field of latestXLMeta. If not we throw read quorum error.
	if count < latestXLMeta.Erasure.DataBlocks {
		// This is the case when we can't reliably deduce object quorum
		return 0, 0, errXLReadQuorum
	}

	// Since all the valid erasure code meta updated at the same time are equivalent, pass dataBlocks
	// from latestXLMeta to get the quorum
	return latestXLMeta.Erasure.DataBlocks, latestXLMeta.Erasure.DataBlocks + 1, nil
}
