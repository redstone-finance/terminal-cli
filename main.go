package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

//go:embed metadata/*.json
var configFS embed.FS

type Config map[string][]string

var (
	mode        string
	exchanges   []string
	tokens      []string
	startDate   string
	endDate     string
	skipConfirm bool
	apiKey      string
)

func main() {
	_ = godotenv.Load()

	rootCmd := &cobra.Command{
		Use:   "terminal-cli",
		Short: "Download crypto trade data from RedStone Terminal",
		Long:  `A CLI tool to batch download trade data (Parquet) for specific exchanges and tokens.`,
		Run:   run,
	}

	// Flags
	rootCmd.Flags().StringVar(&mode, "mode", "day", "Data mode: day (default)")
	rootCmd.Flags().StringSliceVar(&exchanges, "exchanges", []string{}, "Comma-separated list of exchanges (e.g. binance,bybit)")
	rootCmd.Flags().StringSliceVar(&tokens, "tokens", []string{}, "Comma-separated list of token pairs (e.g. btc_usdt,eth_usdc)")
	rootCmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	rootCmd.Flags().StringVar(&endDate, "end-date", "", "End date (YYYY-MM-DD)")
	rootCmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip confirmation prompts")
	rootCmd.Flags().StringVar(&apiKey, "api-key", "", "API Key (overrides API_KEY env var)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	if mode != "day" {
		pterm.Error.Println("Only 'day' mode is currently supported.")
		os.Exit(1)
	}
	if len(exchanges) == 0 || len(tokens) == 0 || startDate == "" {
		cmd.Help()
		pterm.Error.Println("\nMissing required arguments: --exchanges, --tokens, or --start-date")
		os.Exit(1)
	}

	if apiKey == "" {
		apiKey = os.Getenv("API_KEY")
	}

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		pterm.Error.Printf("Invalid start date: %v\n", err)
		os.Exit(1)
	}
	end := start
	if endDate != "" {
		end, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			pterm.Error.Printf("Invalid end date: %v\n", err)
			os.Exit(1)
		}
	}

	config1, _ := loadConfig("metadata/_2025_01_01.json")
	config2, _ := loadConfig("metadata/_2025_10_02.json")
	threshold, _ := time.Parse("2006-01-02", "2025-10-02")

	type Job struct {
		Exchange string
		Pair     string
		Date     time.Time
	}
	var jobs []Job

	curr := start
	for !curr.After(end) {
		var activeConfig Config
		if curr.Before(threshold) {
			activeConfig = config1
		} else {
			activeConfig = config2
		}

		for _, ex := range exchanges {
			ex = strings.TrimSpace(ex)
			if availablePairs, ok := activeConfig[ex]; ok {
				for _, usrPair := range tokens {
					usrPair = strings.TrimSpace(usrPair)
					if contains(availablePairs, usrPair) {
						jobs = append(jobs, Job{
							Exchange: ex,
							Pair:     usrPair,
							Date:     curr,
						})
					}
				}
			}
		}
		curr = curr.AddDate(0, 0, 1)
	}

	if len(jobs) == 0 {
		pterm.Warning.Println("No matching files found for the given criteria.")
		return
	}

	pterm.DefaultSection.Println("Job Summary")
	pterm.Info.Printf("Found %d files to download.\n", len(jobs))
	pterm.Info.Printf("Range: %s to %s\n", jobs[0].Date.Format("2006-01-02"), jobs[len(jobs)-1].Date.Format("2006-01-02"))

	if !skipConfirm {
		result, _ := pterm.DefaultInteractiveConfirm.Show("Do you want to continue?")
		if !result {
			pterm.Warning.Println("Aborted.")
			os.Exit(0)
		}
	}

	pterm.Println()
	successCount := 0
	failCount := 0

	for i, job := range jobs {
		jobTitle := fmt.Sprintf("[%d/%d] %s %s %s", i+1, len(jobs), job.Exchange, job.Pair, job.Date.Format("2006-01-02"))

		spinner, _ := pterm.DefaultSpinner.Start("Fetching link for " + jobTitle)

		dlURL, size, relPath, err := fetchDownloadLink(apiKey, job.Exchange, job.Pair, job.Date)
		if err != nil {
			spinner.Fail(fmt.Sprintf("%s - %v", jobTitle, err))
			failCount++
			continue
		}

		spinner.UpdateText("Downloading " + jobTitle)

		err = downloadFileWithProgress(dlURL, relPath, size, spinner)
		if err != nil {
			spinner.Fail(fmt.Sprintf("%s - Download Error: %v", jobTitle, err))
			failCount++
		} else {
			spinner.Success(jobTitle + " - Saved")
			successCount++
		}
	}

	pterm.Println()
	pterm.Println()
	pterm.Println()
	pterm.DefaultHeader.
		WithBackgroundStyle(pterm.NewStyle(pterm.BgGreen)).
		WithTextStyle(pterm.NewStyle(pterm.FgBlack)).
		Println("Finished")

	summaryTable := pterm.TableData{
		{"Total", fmt.Sprintf("%d", len(jobs))},
		{"Success", fmt.Sprintf("%d", successCount)},
		{"Failed", fmt.Sprintf("%d", failCount)},
	}

	pterm.DefaultTable.WithData(summaryTable).Render()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func loadConfig(path string) (Config, error) {
	data, err := configFS.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return c, nil
}

type APIResponse struct {
	DownloadURL string `json:"download_url"`
	FileSize    int64  `json:"file_size"`
	FilePath    string `json:"file_path"`
	Error       string `json:"error"`
	Message     string `json:"message"`
}

func fetchDownloadLink(apiKey, exchange, pair string, date time.Time) (string, int64, string, error) {
	y, m, d := date.Date()
	dateStr := date.Format("2006-01-02")

	relPath := fmt.Sprintf("%s/trade/%04d/%02d/%02d/%s/%s_trades_%s_%s.parquet",
		exchange, y, m, d, pair, exchange, dateStr, pair)

	baseURL := "https://7879w58k4l.execute-api.eu-west-1.amazonaws.com/dev/"
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", 0, "", err
	}

	q := req.URL.Query()
	q.Add("file", relPath)
	req.URL.RawQuery = q.Encode()

	if apiKey != "" {
		req.Header.Set("x-Api-Key", apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var apiErr APIResponse
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Message != "" {
			return "", 0, "", errors.New(apiErr.Message)
		}
		if resp.StatusCode == 404 {
			return "", 0, "", errors.New("file not found on server")
		}
		return "", 0, "", fmt.Errorf("api status %d", resp.StatusCode)
	}

	var successResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&successResp); err != nil {
		return "", 0, "", fmt.Errorf("invalid json: %v", err)
	}

	return successResp.DownloadURL, successResp.FileSize, relPath, nil
}

func downloadFileWithProgress(url, relPath string, size int64, spinner *pterm.SpinnerPrinter) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	fullPath := filepath.Join("downloads", relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	spinner.Success("Link acquired")

	p, _ := pterm.DefaultProgressbar.WithTotal(int(size)).WithTitle("Downloading").Start()

	proxyReader := &ProgressReader{
		Reader: resp.Body,
		Bar:    p,
	}

	_, err = io.Copy(file, proxyReader)

	_, _ = p.Stop()

	return err
}

type ProgressReader struct {
	Reader io.Reader
	Bar    *pterm.ProgressbarPrinter
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	if n > 0 && pr.Bar != nil {
		pr.Bar.Add(n)
	}
	return n, err
}
