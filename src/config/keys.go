package config

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/spf13/viper"
)

type NetworkBrokerKeysResponse struct {
	ApiiroGatewayPublicKey string
}

func GetServerPublicKey() (string, error) {
	response, err := getNetworkBrokerKeysResponse()
	if err != nil {
		return "", err
	}

	return response.ApiiroGatewayPublicKey, nil
}

func VerifyClientPublicKey(publicKey string) error {
	domain := viper.GetString("Apiiro.Domain")
	params := map[string]string{
		"publicKey": publicKey,
	}

	_, err := sendRequest(fmt.Sprintf("https://%s/rest-api/v1.0/broker/verify", domain), params)
	if err != nil {
		// if response != nil && response.StatusCode == 404 && len(response.Body) < 10 {
		// 	log.Println("Server version doesn't support verify endpoint")
		// 	return nil
		// }
		return err
	}

	return nil
}

func getNetworkBrokerKeysResponse() (NetworkBrokerKeysResponse, error) {
	domain := viper.GetString("Apiiro.Domain")

	response, err := sendRequest(fmt.Sprintf("https://%s/rest-api/v1.0/broker/keys", domain), nil)
	if err != nil {
		return NetworkBrokerKeysResponse{}, err
	}

	// Handle unmarshalled_response
	var unmarshalled_response NetworkBrokerKeysResponse
	err = json.Unmarshal(response.Body, &unmarshalled_response)
	if err != nil {
		log.Printf("Error parsing body, received: %v, %s", err, string(response.Body))
		return NetworkBrokerKeysResponse{}, err
	}

	if unmarshalled_response.ApiiroGatewayPublicKey == "" {
		log.Printf("Response missing gateway public key, received: %s", string(response.Body))
	}

	return unmarshalled_response, err
}

func sendRequest(endpoint string, params map[string]string) (*HttpResponse, error) {
	accessToken := viper.GetString("Config.Token")

	if viper.GetBool("verbose") {
		log.Printf("Sending request to %s\n", endpoint)
	}

	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %s: %v", endpoint, err)
	}

	if params != nil {
		query := reqURL.Query()
		for key, value := range params {
			query.Set(key, value)
		}
		reqURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)

	if err != nil {
		log.Printf("Error setting up request, %v", err)
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("Skip.Ssl.Verify")}

	client := &http.Client{Transport: transport}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error sending request, %v", err)
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response, received: %s", string(body))
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Received status code, %d, %s", resp.StatusCode, string(body))
		return &HttpResponse{body, resp.StatusCode}, fmt.Errorf("%d %s", resp.StatusCode, string(body))
	}

	return &HttpResponse{body, resp.StatusCode}, nil
}

type HttpResponse struct {
	Body       []byte
	StatusCode int
}
