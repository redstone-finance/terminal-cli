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
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

//go:embed all:metadata
var configFS embed.FS

type Config map[string][]string

type ConfigRule struct {
	StartDate time.Time
	Config    Config
}

var (
	mode        string
	dataType    string
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

	rootCmd.Flags().StringVar(&mode, "mode", "day", "Data mode: day (default)")
	rootCmd.Flags().StringVar(&dataType, "type", "trade", "Data type: trade (default), derivative")
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

	configRules, err := loadConfigRules(dataType)
	if err != nil {
		pterm.Error.Printf("Failed to load metadata configurations: %v\n", err)
		os.Exit(1)
	}
	if len(configRules) == 0 {
		pterm.Error.Printf("No configuration files found in metadata/%s folder.\n", dataType)
		os.Exit(1)
	}

	type Job struct {
		Exchange string
		Pair     string
		Date     time.Time
	}
	var jobs []Job

	curr := start
	for !curr.After(end) {
		activeConfig := getConfigForDate(configRules, curr)

		if activeConfig != nil {
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
		}
		curr = curr.AddDate(0, 0, 1)
	}

	if len(jobs) == 0 {
		pterm.Warning.Println("No matching files found for the given criteria.")
		return
	}

	pterm.DefaultSection.Println("Job Summary")
	pterm.Info.Printf("Type: %s\n", dataType)
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
		relPath := getRelativePath(job.Exchange, job.Pair, dataType, job.Date)
		displayPath := filepath.Join("downloads", relPath)

		jobLabel := fmt.Sprintf("[%d/%d] %s", i+1, len(jobs), displayPath)

		spinner, _ := pterm.DefaultSpinner.Start(jobLabel + " ... Fetching Link")

		dlURL, size, err := fetchDownloadLink(apiKey, relPath)
		if err != nil {
			spinner.Fail(fmt.Sprintf("%s - %v", jobLabel, err))
			failCount++
			continue
		}

		spinner.RemoveWhenDone = true
		_ = spinner.Stop()

		barTitle := fmt.Sprintf("%s %s", pterm.LightBlue("LOADING"), jobLabel)

		err = downloadFileWithProgress(dlURL, relPath, size, barTitle)

		if err != nil {
			pterm.Error.Printf("%s - %v\n", jobLabel, err)
			failCount++
		} else {
			sizeStr := pterm.Gray(fmt.Sprintf("(%.2f MB)", float64(size)/1024/1024))
			pterm.Success.Printf("%s - Saved %s\n", jobLabel, sizeStr)
			successCount++
		}
	}

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

func loadConfigRules(dType string) ([]ConfigRule, error) {
	dirPath := "metadata/" + dType
	entries, err := configFS.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("could not read directory %s: %v", dirPath, err)
	}

	var rules []ConfigRule
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		name = strings.TrimPrefix(name, "_")

		date, err := time.Parse("2006_01_02", name)
		if err != nil {
			date, err = time.Parse("2006-01-02", name)
			if err != nil {
				continue
			}
		}

		content, err := configFS.ReadFile(filepath.Join(dirPath, entry.Name()))
		if err != nil {
			return nil, err
		}
		var cfg Config
		if err := json.Unmarshal(content, &cfg); err != nil {
			return nil, fmt.Errorf("invalid json in %s: %v", entry.Name(), err)
		}
		rules = append(rules, ConfigRule{StartDate: date, Config: cfg})
	}

	sort.Slice(rules, func(i, j int) bool {
		return rules[i].StartDate.Before(rules[j].StartDate)
	})
	return rules, nil
}

func getConfigForDate(rules []ConfigRule, date time.Time) Config {
	for i := len(rules) - 1; i >= 0; i-- {
		if !date.Before(rules[i].StartDate) {
			return rules[i].Config
		}
	}
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func getRelativePath(exchange, pair, dType string, date time.Time) string {
	y, m, d := date.Date()
	dateStr := date.Format("2006-01-02")

	var folderPart, filePart string
	if dType == "trade" {
		folderPart = "trade"
		filePart = "trades"
	} else {
		folderPart = dType
		filePart = dType
	}

	return fmt.Sprintf("%s/%s/%04d/%02d/%02d/%s/%s_%s_%s_%s.parquet",
		exchange, folderPart, y, m, d, pair, exchange, filePart, dateStr, pair)
}

type APIResponse struct {
	DownloadURL string `json:"download_url"`
	FileSize    int64  `json:"file_size"`
	FilePath    string `json:"file_path"`
	Error       string `json:"error"`
	Message     string `json:"message"`
}

func fetchDownloadLink(apiKey, relPath string) (string, int64, error) {
	baseURL := "https://7879w58k4l.execute-api.eu-west-1.amazonaws.com/dev/"
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return "", 0, err
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
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var apiErr APIResponse
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if apiErr.Message != "" {
			return "", 0, errors.New(apiErr.Message)
		}
		if resp.StatusCode == 404 {
			return "", 0, errors.New("file not found on server")
		}
		return "", 0, fmt.Errorf("api status %d", resp.StatusCode)
	}

	var successResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&successResp); err != nil {
		return "", 0, fmt.Errorf("invalid json: %v", err)
	}

	return successResp.DownloadURL, successResp.FileSize, nil
}

func downloadFileWithProgress(url, relPath string, size int64, barTitle string) error {
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

	p, _ := pterm.DefaultProgressbar.
		WithTotal(int(size)).
		WithTitle(barTitle).
		WithRemoveWhenDone(true).
		Start()

	proxyReader := &ProgressReader{Reader: resp.Body, Bar: p}
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
