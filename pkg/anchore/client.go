// Copyright © 2019 Banzai Cloud.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package anchore

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/banzaicloud/anchore-image-validator/pkg/apis/security/v1alpha1"
	"github.com/docker/distribution/reference"
	"github.com/sirupsen/logrus"
)

func anchoreRequest(path string, bodyParams map[string]string, method string) ([]byte, error) {
	username := os.Getenv("ANCHORE_ENGINE_USERNAME")
	password := os.Getenv("ANCHORE_ENGINE_PASSWORD")
	anchoreEngineURL := os.Getenv("ANCHORE_ENGINE_URL")
	fullURL := anchoreEngineURL + path

	var insecure bool
	if os.Getenv("ANCHORE_ENGINE_INSECURE") == "true" {
		insecure = true
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
			},
		},
	}

	bodyParamJSON, err := json.Marshal(bodyParams)

	if err != nil {
		logrus.Fatal(err)
	}

	req, err := http.NewRequest(method, fullURL, bytes.NewBuffer(bodyParamJSON))

	if err != nil {
		logrus.Fatal(err)
	}

	req.SetBasicAuth(username, password)

	logrus.WithFields(logrus.Fields{
		"url":        fullURL,
		"bodyParams": bodyParams,
	}).Info("Sending request")

	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("failed to complete request to Anchore: %v", err)
	}

	bodyText, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

	logrus.WithFields(logrus.Fields{
		"response": string(bodyText),
	}).Info("Anchore Response Body")

	if err != nil {
		return nil, fmt.Errorf("failed to complete request to Anchore: %v", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("response from Anchore: %d", resp.StatusCode)
	}

	return bodyText, nil
}

func getStatus(digest string, tag string) bool {
	path := fmt.Sprintf("/v1/images/%s/check?history=false&detail=false&tag=%s", digest, tag)
	body, err := anchoreRequest(path, nil, "GET")

	if err != nil {
		logrus.Error(err)
		return false
	}

	var result []map[string]map[string][]SHAResult
	err = json.Unmarshal(body, &result)

	if err != nil {
		logrus.Error(err)
		return false
	}

	resultIndex := fmt.Sprintf("docker.io/%s:latest", tag)

	return result[0][digest][resultIndex][0].Status == "pass"
}

func getOrAddImage(imageRef string) (Image, error) {
	params := map[string]string{"tag": imageRef}
	body, err := anchoreRequest("/v1/images?history=false", params, "POST")

	if err != nil {
		return Image{}, err
	}

	var images []Image
	err = json.Unmarshal(body, &images)

	if err != nil {
		return Image{}, fmt.Errorf("failed to unmarshal JSON from response: %v", err)
	}

	logrus.WithFields(logrus.Fields{
		"Image": images[0],
	}).Info("Get or Added image")

	return images[0], nil
}

func getImageDigest(imageRef string) (string, error) {
	image, err := getOrAddImage(imageRef)

	if err != nil {
		return "", fmt.Errorf("failed to get image digest: %v", err)
	}

	return image.ImageDigest, nil
}

//CheckImage checking Image with Anchore
func CheckImage(image string) (v1alpha1.AuditImage, bool) {
	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		logrus.Error(err)
		return v1alpha1.AuditImage{}, false
	}
	imageName := ref.(reference.Named).Name()
	imageTag := reference.TagNameOnly(ref).(reference.Tagged).Tag()

	logrus.WithFields(logrus.Fields{
		"image_name": imageName,
		"image_tag":  imageTag,
	}).Info("Checking image")

	digest, err := getImageDigest(image)

	if err != nil {
		return v1alpha1.AuditImage{}, false
	}

	lastUpdated := getImageLastUpdate(digest)
	auditImage := v1alpha1.AuditImage{
		ImageName:   imageName,
		ImageTag:    imageTag,
		ImageDigest: digest,
		LastUpdated: lastUpdated,
	}

	return auditImage, getStatus(digest, imageTag)
}

func getImageLastUpdate(digest string) string {
	path := fmt.Sprintf("/v1/images/%s?history=false&detail=false", digest)
	body, err := anchoreRequest(path, nil, "GET")

	if err != nil {
		logrus.Error(err)
		return ""
	}

	var images []Image
	err = json.Unmarshal(body, &images)

	if err != nil {
		logrus.Error(err)
		return ""
	}

	return images[0].LastUpdated
}
