package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/gorilla/mux"
)

const notificationsApi = "http://localhost:3000/"

type Request struct {
	Method      string `json:"method"`
	RelativeUrl string `json:"relative_url,omitempty"`
	Body        string `json:"body,omitempty"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Response struct {
	Code    int `json:"code"`
	Headers []Header
	Body    string `json:"body"`
}

type BatchResponse struct {
	Code int        `json:"code"`
	Body []Response `json:"body"`
}

type Requests []Request

type BatchRequest struct {
	AccessToken string   `json:"access_token"`
	Batch       Requests `json:"batch"`
}

func decodeRequests(req *http.Request) (batch BatchRequest, err error) {
	decoder := json.NewDecoder(req.Body)
	var batchRequest BatchRequest
	decode_err := decoder.Decode(&batchRequest)
	if decode_err != nil {
		return batchRequest, decode_err
	}
	return batchRequest, nil
}

func MakeRequest(client http.Client, request Request, ch chan<- Response) {
	var method = request.Method
	var relative_url = request.RelativeUrl

	url := fmt.Sprintf("%s%s", notificationsApi, relative_url)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("Authorization", "Basic dXNlcm5hbWU6cGFzc3dvcmQ=")

	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	jsonParsed, err := gabs.ParseJSON(body)
	resp := Response{Code: res.StatusCode, Body: jsonParsed.String()}

	ch <- resp
}

func CreateBatchEndpoint(w http.ResponseWriter, req *http.Request) {
	batchRequest, err := decodeRequests(req)
	if err != nil {
		log.Fatal(err)
	}

	apiClient := http.Client{Timeout: time.Second * 10}

	var responses []Response

	start := time.Now()
	ch := make(chan Response)

	for _, request := range batchRequest.Batch {
		go MakeRequest(apiClient, request, ch)
	}

	for range batchRequest.Batch {
		responses = append(responses, <-ch)
	}
	fmt.Printf("%.2fs elapsed\n", time.Since(start).Seconds())

	json.NewEncoder(w).Encode(BatchResponse{Code: 200, Body: responses})
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/", CreateBatchEndpoint).Methods("POST")
	log.Fatal(http.ListenAndServe(":2345", router))
}
