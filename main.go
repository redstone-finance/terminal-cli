package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
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
	parallelism int
)

func main() {
	_ = godotenv.Load()

	rootCmd := &cobra.Command{
		Use:   "terminal-cli",
		Short: "Download crypto trade data from RedStone Terminal",
		Long:  `A CLI tool to batch download trade data (Parquet) for specific exchanges and tokens.`,
		Run:   run,
	}

	rootCmd.Flags().StringVar(&mode, "mode", "day", "Data mode: day, check")
	rootCmd.Flags().StringVar(&dataType, "type", "trade", "Data type: trade (default), derivative")
	rootCmd.Flags().StringSliceVar(&exchanges, "exchanges", []string{}, "Comma-separated list of exchanges")
	rootCmd.Flags().StringSliceVar(&tokens, "tokens", []string{}, "Comma-separated list of token pairs")
	rootCmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	rootCmd.Flags().StringVar(&endDate, "end-date", "", "End date (YYYY-MM-DD)")
	rootCmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip confirmation prompts")
	rootCmd.Flags().StringVar(&apiKey, "api-key", "", "API Key (overrides API_KEY env var)")
	rootCmd.Flags().IntVarP(&parallelism, "parallel", "p", 10, "Number of parallel downloads")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type Job struct {
	Index    int
	Total    int
	Exchange string
	Pair     string
	Date     time.Time
	Bar      *pterm.ProgressbarPrinter
}

func run(cmd *cobra.Command, args []string) {
	if startDate == "" {
		cmd.Help()
		pterm.Error.Println("\nMissing required argument: --start-date")
		os.Exit(1)
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
		if end.Before(start) {
			pterm.Error.Printf("--end-date (%s) is before --start-date (%s)\n", endDate, startDate)
			os.Exit(1)
		}
	}

	exchanges = normalize(exchanges)
	tokens = normalize(tokens)

	configRules, err := loadConfigRules(dataType)
	if err != nil {
		pterm.Error.Printf("Failed to load metadata configurations: %v\n", err)
		os.Exit(1)
	}
	if len(configRules) == 0 {
		pterm.Error.Printf("No configuration files found in metadata/%s folder.\n", dataType)
		os.Exit(1)
	}

	switch mode {
	case "check":
		runCheckMode(start, end, configRules)
	case "day":
		if len(exchanges) == 0 || len(tokens) == 0 {
			pterm.Error.Println("\nMode 'day' requires: --exchanges and --tokens")
			os.Exit(1)
		}
		if parallelism < 1 {
			pterm.Error.Printf("--parallel must be at least 1, got %d\n", parallelism)
			os.Exit(1)
		}
		if apiKey == "" {
			apiKey = os.Getenv("API_KEY")
		}
		if apiKey == "" {
			pterm.Error.Println("\nMissing API key: pass --api-key, or set API_KEY in the environment or a .env file")
			os.Exit(1)
		}
		runDayMode(start, end, configRules)
	default:
		pterm.Error.Printf("Unknown mode: %s. Supported modes: day, check\n", mode)
		os.Exit(1)
	}
}

type AvailabilityBlock struct {
	Start, End time.Time
	Data       map[string][]string // Exchange -> Tokens
}

func runCheckMode(start, end time.Time, configRules []ConfigRule) {
	pterm.DefaultSection.Println("Checking Data Availability")
	pterm.Info.Printf("Type:  %s\n", dataType)
	pterm.Info.Printf("Range: %s to %s\n", start.Format("2006-01-02"), end.Format("2006-01-02"))
	pterm.Println()

	var blocks []AvailabilityBlock
	var currentBlock *AvailabilityBlock

	curr := start
	for !curr.After(end) {
		activeConfig := getConfigForDate(configRules, curr)

		dayData := make(map[string][]string)
		hasData := false

		if activeConfig != nil {
			for ex, pairs := range activeConfig {
				if len(exchanges) > 0 && !slices.Contains(exchanges, ex) {
					continue
				}

				var validPairs []string
				for _, pair := range pairs {
					if len(tokens) > 0 && !slices.Contains(tokens, pair) {
						continue
					}
					validPairs = append(validPairs, pair)
				}
				sort.Strings(validPairs)

				if len(validPairs) > 0 {
					dayData[ex] = validPairs
					hasData = true
				}
			}
		}

		if currentBlock == nil {
			if hasData {
				currentBlock = &AvailabilityBlock{Start: curr, End: curr, Data: dayData}
			}
		} else {
			if maps.EqualFunc(currentBlock.Data, dayData, slices.Equal) {
				currentBlock.End = curr
			} else {
				blocks = append(blocks, *currentBlock)
				if hasData {
					currentBlock = &AvailabilityBlock{Start: curr, End: curr, Data: dayData}
				} else {
					currentBlock = nil
				}
			}
		}
		curr = curr.AddDate(0, 0, 1)
	}

	if currentBlock != nil {
		blocks = append(blocks, *currentBlock)
	}

	if len(blocks) == 0 {
		pterm.Warning.Println("No data found for the specified criteria.")
		return
	}

	for _, block := range blocks {
		periodStr := fmt.Sprintf("Period: %s to %s",
			block.Start.Format("2006-01-02"),
			block.End.Format("2006-01-02"))

		pterm.DefaultHeader.WithBackgroundStyle(pterm.NewStyle(pterm.BgBlue)).Println(periodStr)

		tableData := [][]string{{"Exchange", "Available Tokens"}}

		var sortedExs []string
		for ex := range block.Data {
			sortedExs = append(sortedExs, ex)
		}
		sort.Strings(sortedExs)

		for i, ex := range sortedExs {
			tokensStr := strings.Join(block.Data[ex], ", ")
			tokensStr = wordWrap(tokensStr, 80)

			tableData = append(tableData, []string{ex, tokensStr})

			if i < len(sortedExs)-1 {
				tableData = append(tableData, []string{"", ""})
			}
		}

		pterm.DefaultTable.
			WithHasHeader().
			WithBoxed().
			WithData(tableData).
			Render()

		pterm.Println()
	}
}

func wordWrap(text string, lineWidth int) string {
	words := strings.Split(text, " ")
	if len(words) == 0 {
		return text
	}
	wrapped := words[0]
	spaceLeft := lineWidth - len(wrapped)
	for _, word := range words[1:] {
		if len(word)+1 > spaceLeft {
			wrapped += "\n" + word
			spaceLeft = lineWidth - len(word)
		} else {
			wrapped += " " + word
			spaceLeft -= 1 + len(word)
		}
	}
	return wrapped
}

func runDayMode(start, end time.Time, configRules []ConfigRule) {
	var jobs []Job
	curr := start
	for !curr.After(end) {
		activeConfig := getConfigForDate(configRules, curr)
		if activeConfig != nil {
			for _, ex := range exchanges {
				if availablePairs, ok := activeConfig[ex]; ok {
					for _, usrPair := range tokens {
						if slices.Contains(availablePairs, usrPair) {
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

	for i := range jobs {
		jobs[i].Index = i + 1
		jobs[i].Total = len(jobs)
	}

	pterm.DefaultSection.Println("Job Summary")
	pterm.Info.Printf("Type: %s\n", dataType)
	pterm.Info.Printf("Count: %d files\n", len(jobs))
	pterm.Info.Printf("Concurrency: %d\n", parallelism)
	pterm.Info.Printf("Range: %s to %s\n", jobs[0].Date.Format("2006-01-02"), jobs[len(jobs)-1].Date.Format("2006-01-02"))

	if !skipConfirm {
		result, _ := pterm.DefaultInteractiveConfirm.Show("Do you want to continue?")
		if !result {
			pterm.Warning.Println("Aborted.")
			os.Exit(0)
		}
	}

	pterm.Println()
	runDownloads(jobs)
}

func runDownloads(jobs []Job) {
	multi := pterm.DefaultMultiPrinter

	// Every writer must exist before Start: NewWriter appends to
	// multi.buffers, which Start's 200ms render ticker then reads.
	for i := range jobs {
		relPath := getRelativePath(jobs[i].Exchange, jobs[i].Pair, dataType, jobs[i].Date)
		fullPath := localPath(relPath)
		jobLabel := fmt.Sprintf("[%d/%d] %s", jobs[i].Index, jobs[i].Total, fullPath)

		bar, _ := pterm.DefaultProgressbar.
			WithWriter(multi.NewWriter()).
			WithTotal(100).
			WithTitle(fmt.Sprintf("%s ... Pending", jobLabel)).
			Start()

		jobs[i].Bar = bar
	}

	multi.Start()

	jobsCh := make(chan Job, len(jobs))
	var wg sync.WaitGroup

	var successCount, failCount, skipCount int64
	var mu sync.Mutex

	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsCh {
				processJob(job, &successCount, &failCount, &skipCount, &mu)
			}
		}()
	}

	for _, j := range jobs {
		jobsCh <- j
	}
	close(jobsCh)

	wg.Wait()
	multi.Stop()

	pterm.Println()
	pterm.DefaultHeader.
		WithBackgroundStyle(pterm.NewStyle(pterm.BgGreen)).
		WithTextStyle(pterm.NewStyle(pterm.FgBlack)).
		Println("Finished")

	row := func(label string, val int64, style *pterm.Style) []string {
		return []string{style.Sprint(label), style.Sprint(fmt.Sprintf("%d", val))}
	}

	summaryTable := pterm.TableData{
		row("Total", int64(len(jobs)), pterm.NewStyle(pterm.FgLightBlue)),
		row("Success", successCount, pterm.NewStyle(pterm.FgGreen)),
		row("Skipped", skipCount, pterm.NewStyle(pterm.FgYellow)),
		row("Failed", failCount, pterm.NewStyle(pterm.FgRed)),
	}
	pterm.DefaultTable.WithData(summaryTable).Render()
}

func processJob(job Job, success, fail, skip *int64, mu *sync.Mutex) {
	relPath := getRelativePath(job.Exchange, job.Pair, dataType, job.Date)
	fullPath := localPath(relPath)
	jobLabel := fmt.Sprintf("[%d/%d] %s", job.Index, job.Total, fullPath)

	errPrefix := pterm.Error.Prefix.Style.Sprint(pterm.Error.Prefix.Text)
	okPrefix := pterm.Success.Prefix.Style.Sprint(pterm.Success.Prefix.Text)
	skipPrefix := pterm.Warning.Prefix.Style.Sprint(pterm.Warning.Prefix.Text)

	bar := job.Bar

	if _, err := os.Stat(fullPath); err == nil {
		bar.UpdateTitle(fmt.Sprintf("%s %s - Skipped (Exists)", skipPrefix, jobLabel))
		bar.Total = 1
		bar.Increment()
		_, _ = bar.Stop()

		mu.Lock()
		*skip++
		mu.Unlock()
		return
	}

	bar.UpdateTitle(fmt.Sprintf("%s %s ... Fetching", pterm.LightBlue("LOADING"), jobLabel))
	dlURL, size, err := fetchDownloadLink(apiKey, relPath)
	if err != nil {
		bar.UpdateTitle(fmt.Sprintf("%s %s - Error: %v", errPrefix, jobLabel, err))
		_, _ = bar.Stop()
		mu.Lock()
		*fail++
		mu.Unlock()
		return
	}

	bar.UpdateTitle(fmt.Sprintf("%s %s", pterm.LightBlue("LOADING"), jobLabel))
	// +1 keeps the bar out of pterm's two dead states: Total==0 renders an
	// empty string (the job's row vanishes), and Current==Total auto-stops the
	// bar, after which UpdateTitle also renders an empty string and the final
	// "Saved" line is never shown.
	bar.Total = int(size) + 1

	err = downloadStream(dlURL, fullPath, bar)

	if err != nil {
		bar.UpdateTitle(fmt.Sprintf("%s %s - Failed: %v", errPrefix, jobLabel, err))
		_, _ = bar.Stop()
		mu.Lock()
		*fail++
		mu.Unlock()
	} else {
		sizeStr := pterm.Gray(fmt.Sprintf("(%.2f MB)", float64(size)/1024/1024))
		successMsg := fmt.Sprintf("%s %s - Saved %s", okPrefix, jobLabel, sizeStr)
		bar.UpdateTitle(successMsg)
		_, _ = bar.Stop()
		mu.Lock()
		*success++
		mu.Unlock()
	}
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

		// path.Join, not filepath.Join: io/fs paths are always slash-separated,
		// so the backslashes filepath produces on Windows never resolve.
		content, err := configFS.ReadFile(path.Join(dirPath, entry.Name()))
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

// pflag parses string slices with a CSV reader that keeps leading spaces, so
// --exchanges "binance, bybit" yields [binance, " bybit"]. Duplicates matter
// too: they used to produce two jobs writing the same file concurrently.
func normalize(values []string) []string {
	var out []string
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
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

// Windows rejects these in file and directory names; derivative pairs are all
// of the form "perp:venue:base_quote". Only the local path is rewritten - the
// API is still asked for the original relPath.
const reservedPathChars = `<>:"|?*`

func localPath(relPath string) string {
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune(reservedPathChars, r) {
			return '_'
		}
		return r
	}, relPath)
	return filepath.Join("downloads", safe)
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
		// The gateway uses "message" for some failures and "error" for others.
		for _, msg := range []string{apiErr.Message, apiErr.Error} {
			if msg != "" {
				return "", 0, errors.New(msg)
			}
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

// http.DefaultClient has no timeout, so a stalled S3 connection would hang a
// worker (and, at -p 1, the whole CLI) forever.
// Whole-request cap rather than an idle-read deadline; raise it if single
// files ever outgrow it.
var downloadClient = &http.Client{Timeout: 30 * time.Minute}

func downloadStream(url, fullPath string, bar *pterm.ProgressbarPrinter) error {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}
	// Download to a temporary file so an interrupted transfer never leaves a
	// truncated .parquet that the "already exists" check would skip forever.
	tmpPath := fullPath + ".part"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	proxyReader := &ProgressReader{Reader: resp.Body, Bar: bar}
	if _, err := io.Copy(file, proxyReader); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, fullPath)
}

// Every Bar.Add re-renders the bar into its multi-printer buffer, which is
// never truncated: at one render per 32KB read a 1GB file left ~10MB of ANSI
// behind, and the multi-printer re-scans every buffer 5 times a second.
const progressUpdateInterval = 100 * time.Millisecond

type ProgressReader struct {
	Reader io.Reader
	Bar    *pterm.ProgressbarPrinter

	pending    int
	lastUpdate time.Time
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.pending += n

	if pr.Bar != nil && pr.pending > 0 && (err != nil || time.Since(pr.lastUpdate) >= progressUpdateInterval) {
		pr.Bar.Add(pr.pending)
		pr.pending = 0
		pr.lastUpdate = time.Now()
	}

	return n, err
}
