package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	hdrhistogram "github.com/HdrHistogram/hdrhistogram-go"
)

const url = "http://localhost:8080/"

func main() {
	file, err := os.OpenFile("put.txt", os.O_RDONLY, 0o644)
	if err != nil {
		log.Printf("couldn't open file: %v", err)
		return
	}
	defer file.Close()

	hist := hdrhistogram.New(1, 60000, 3)

	scanner := bufio.NewScanner(file)
	var errorCount int

	for scanner.Scan() {
		line := scanner.Text()
		err = makeRequest(hist, line)
		if err != nil {
			errorCount++
			fmt.Printf("couldn't process request: %v\n", err)
			continue
		}
	}

	fmt.Println("--- Request Latency Metrics (ms) ---")
	fmt.Printf("p50 (Median): %d ms\n", hist.ValueAtQuantile(50))
	fmt.Printf("p95:          %d ms\n", hist.ValueAtQuantile(95))
	fmt.Printf("p99:          %d ms\n", hist.ValueAtQuantile(99))

	fmt.Println("error count: ", errorCount)
}

func makeRequest(hdrhist *hdrhistogram.Histogram, requestLine string) error {
	start := time.Now()

	defer func() {
		duration := time.Since(start).Milliseconds()
		hdrhist.RecordValue(duration)
	}()

	parts := strings.Split(requestLine, " ")
	if len(parts) != 3 {
		return fmt.Errorf("invalid request line")
	}

	if parts[0] == "PUT" {
		req, err := http.NewRequest(http.MethodPut, url+parts[1], strings.NewReader(parts[2]))
		if err != nil {
			return err
		}

		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(res.Body)
			return fmt.Errorf("an error occured performing PUT action: %s", string(data))
		}
	}

	if parts[0] == "GET" {
		req, err := http.NewRequest(http.MethodGet, url+parts[1], nil)
		if err != nil {
			return err
		}

		client := &http.Client{Timeout: 5 * time.Second}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()

		if parts[2] == "NOT_FOUND" && res.StatusCode == http.StatusNotFound {
			return nil
		}

		data, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("an error occured performing GET action: %s", string(data))
		}

		if string(data) != parts[2] {
			return fmt.Errorf("wrong value for the given key. key: %s, returned value: %s, actual value: %s", parts[1], string(data), parts[2])
		}
	}

	return nil
}
