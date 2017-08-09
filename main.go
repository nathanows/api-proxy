package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/gorilla/mux"
)

const baseApiUrl = "http://localhost:3000/"

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
	Headers http.Header
	Body    json.RawMessage `json:"body"`
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

func MakeRequest(client *http.Client, request Request) (response Response) {
	var method = request.Method
	var relative_url = request.RelativeUrl

	url := fmt.Sprintf("%s%s", baseApiUrl, relative_url)
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	jsonParsed, err := gabs.ParseJSON(body)
	return Response{Code: res.StatusCode, Headers: res.Header, Body: json.RawMessage(jsonParsed.String())}
}

type RoundTripFunc func(*http.Request) (*http.Response, error)

func (rt RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt(r)
}

// LimitConcurrency limits how many requests can be processed at once
func LimitConcurrency(rt http.RoundTripper, limit int) http.RoundTripper {
	limiter := make(chan struct{}, limit)
	push := func() {
		start := time.Now()
		limiter <- struct{}{}
		fmt.Println("time spent waiting:", time.Since(start))
	}
	pop := func() {
		<-limiter
	}
	return RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		// reserve a slot
		push()
		// free the slot back up
		defer pop()

		// use the given round tripper
		return rt.RoundTrip(r)
	})
}

// Delay the round-trip for some duration
func Delay(rt http.RoundTripper, d time.Duration) http.RoundTripper {
	return RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		time.Sleep(d)
		return rt.RoundTrip(r)
	})
}

func CreateBatchEndpoint(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	batchRequest, err := decodeRequests(req)
	if err != nil {
		log.Fatal(err)
	}

	var responses []Response

	delay := time.Millisecond * 100
	delayed := Delay(http.DefaultTransport, delay)
	apiClient := &http.Client{
		Transport: LimitConcurrency(delayed, 10),
		Timeout:   time.Second * 30,
	}

	wg := sync.WaitGroup{}
	for _, request := range batchRequest.Batch {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := MakeRequest(apiClient, request)
			responses = append(responses, resp)
		}()
	}
	wg.Wait()

	fmt.Printf("%.2fs elapsed\n", time.Since(start).Seconds())

	json.NewEncoder(w).Encode(BatchResponse{Code: 200, Body: responses})
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/", CreateBatchEndpoint).Methods("POST")
	log.Fatal(http.ListenAndServe(":2345", router))
}
