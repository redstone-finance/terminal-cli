package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net"
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
	silent      bool
	apiKey      string
	parallelism int
)

const (
	exitFailure = 1
	exitUsage   = 2
	// Finished without errors, but the server does not have some files yet.
	exitMissing = 3
	exitNoMatch = 4
	exitAuth    = 5
	// Retrying later may succeed.
	exitNetwork = 6
	exitDisk    = 7
)

// Set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	_ = godotenv.Load()

	rootCmd := &cobra.Command{
		Use:   "terminal-cli",
		Short: "Download crypto trade data from RedStone Terminal",
		Long:  `A CLI tool to batch download trade data (Parquet) for specific exchanges and tokens.`,
		Example: `  # API key (or put it in a .env file)
  export REDSTONE_TERMINAL_API_KEY=your_secret_key_here

  # RedStone live prices (ticker data)
  terminal-cli --type ticker --exchanges redstonelive --tokens btc_usd,eth_usd --start-date 2026-09-01 --end-date 2026-09-07

  # Trades from several exchanges
  terminal-cli --exchanges binance,bingx,cryptocom --tokens btc_usdt,eth_usdt --start-date 2026-09-01 --end-date 2026-09-07

  # See what is available before downloading
  terminal-cli --mode check --type ticker --start-date 2026-09-01`,
		Run:     run,
		Version: version,
	}

	rootCmd.Flags().StringVar(&mode, "mode", "day", "Data mode: day, check")
	rootCmd.Flags().StringVar(&dataType, "type", "trade", "Data type: trade (default), derivative")
	rootCmd.Flags().StringSliceVar(&exchanges, "exchanges", []string{}, "Comma-separated list of exchanges")
	rootCmd.Flags().StringSliceVar(&tokens, "tokens", []string{}, "Comma-separated list of token pairs")
	rootCmd.Flags().StringVar(&startDate, "start-date", "", "Start date (YYYY-MM-DD)")
	rootCmd.Flags().StringVar(&endDate, "end-date", "", "End date (YYYY-MM-DD)")
	rootCmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip confirmation prompts")
	rootCmd.Flags().StringVar(&apiKey, "api-key", "", "API Key (overrides REDSTONE_TERMINAL_API_KEY env var)")
	rootCmd.Flags().IntVarP(&parallelism, "parallel", "p", 10, "Number of parallel downloads")
	rootCmd.Flags().BoolVarP(&silent, "silent", "s", false, "Print nothing and skip prompts (implies --yes); rely on the exit status")

	// Only flags parsed before the bad one are set, so this sees --silent only if it came first.
	rootCmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		c.SilenceErrors, c.SilenceUsage = silent, silent
		return err
	})
	// Run never returns an error, so anything here is a flag or argument error.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitUsage)
	}
}

type Job struct {
	Exchange string
	Pair     string
	Date     time.Time
}

func run(cmd *cobra.Command, args []string) {
	if silent {
		pterm.DisableOutput()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		skipConfirm = true
	}

	if startDate == "" {
		cmd.Help()
		pterm.Error.Println("\nMissing required argument: --start-date")
		os.Exit(exitUsage)
	}

	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		pterm.Error.Printf("Invalid start date: %v\n", err)
		os.Exit(exitUsage)
	}
	end := start
	if endDate != "" {
		end, err = time.Parse("2006-01-02", endDate)
		if err != nil {
			pterm.Error.Printf("Invalid end date: %v\n", err)
			os.Exit(exitUsage)
		}
		if end.Before(start) {
			pterm.Error.Printf("--end-date (%s) is before --start-date (%s)\n", endDate, startDate)
			os.Exit(exitUsage)
		}
	}

	exchanges = normalize(exchanges)
	tokens = normalize(tokens)

	configRules, err := loadConfigRules(dataType)
	if err != nil {
		pterm.Error.Printf("Failed to load metadata configurations: %v\n", err)
		os.Exit(exitUsage)
	}
	if len(configRules) == 0 {
		pterm.Error.Printf("No configuration files found in metadata/%s folder.\n", dataType)
		os.Exit(exitUsage)
	}

	switch mode {
	case "check":
		runCheckMode(start, end, configRules)
	case "day":
		if len(exchanges) == 0 || len(tokens) == 0 {
			pterm.Error.Println("\nMode 'day' requires: --exchanges and --tokens")
			os.Exit(exitUsage)
		}
		if parallelism < 1 {
			pterm.Error.Printf("--parallel must be at least 1, got %d\n", parallelism)
			os.Exit(exitUsage)
		}
		if apiKey == "" {
			apiKey = os.Getenv("REDSTONE_TERMINAL_API_KEY")
		}
		if apiKey == "" {
			pterm.Error.Println("\nMissing API key: pass --api-key, or set REDSTONE_TERMINAL_API_KEY in the environment or a .env file")
			os.Exit(exitUsage)
		}
		runDayMode(start, end, configRules)
	default:
		pterm.Error.Printf("Unknown mode: %s. Supported modes: day, check\n", mode)
		os.Exit(exitUsage)
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
		os.Exit(exitNoMatch)
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
		os.Exit(exitNoMatch)
	}

	pterm.DefaultSection.Println("Job Summary")
	pterm.Info.Printf("Type: %s\n", dataType)
	pterm.Info.Printf("Count: %d files\n", len(jobs))
	pterm.Info.Printf("Concurrency: %d\n", parallelism)
	pterm.Info.Printf("Range: %s to %s\n", jobs[0].Date.Format("2006-01-02"), jobs[len(jobs)-1].Date.Format("2006-01-02"))

	if today := time.Now().UTC().Truncate(24 * time.Hour); !jobs[len(jobs)-1].Date.Before(today) {
		pterm.Warning.Println("Range reaches today: today's files are usually not published yet")
	}

	if !skipConfirm {
		result, _ := pterm.DefaultInteractiveConfirm.Show("Do you want to continue?")
		if !result {
			pterm.Warning.Println("Aborted.")
			os.Exit(exitFailure)
		}
	}

	pterm.Println()
	runDownloads(jobs)
}

type outcome int

// Ordered by severity: the worst outcome picks the exit status, so a failure
// that retrying cannot fix outranks one it can.
const (
	saved outcome = iota
	skipped
	missing
	failedNetwork
	failed
	failedDisk
	failedAuth
)

var exitStatus = [...]int{
	missing:       exitMissing,
	failedNetwork: exitNetwork,
	failed:        exitFailure,
	failedDisk:    exitDisk,
	failedAuth:    exitAuth,
}

func runDownloads(jobs []Job) {
	var bar *pterm.ProgressbarPrinter
	// Start and Stop write cursor escapes straight to stdout, ignoring DisableOutput.
	if !silent {
		bar, _ = pterm.DefaultProgressbar.
			WithTotal(len(jobs)).
			WithTitle("Downloading").
			WithShowElapsedTime(false).
			WithRemoveWhenDone(true).
			Start()
	}

	jobsCh := make(chan Job, len(jobs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var counts [failedAuth + 1]int
	worst := saved

	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobsCh {
				o, line := processJob(job)
				// Serialised: pterm's print-over-active-bar redraw is not goroutine safe.
				mu.Lock()
				counts[o]++
				worst = max(worst, o)
				if line != "" {
					pterm.Println(line)
				}
				if bar != nil {
					bar.Increment()
				}
				mu.Unlock()
			}
		}()
	}

	for _, j := range jobs {
		jobsCh <- j
	}
	close(jobsCh)

	wg.Wait()
	if bar != nil {
		_, _ = bar.Stop()
	}

	pterm.Println()
	row := func(label string, val int, style *pterm.Style) []string {
		return []string{style.Sprint(label), style.Sprint(val)}
	}
	_ = pterm.DefaultTable.WithData(pterm.TableData{
		row("Total", len(jobs), pterm.NewStyle(pterm.FgLightBlue)),
		row("Saved", counts[saved], pterm.NewStyle(pterm.FgGreen)),
		row("Skipped", counts[skipped], pterm.NewStyle(pterm.FgYellow)),
		row("Missing", counts[missing], pterm.NewStyle(pterm.FgYellow)),
		row("Failed", len(jobs)-counts[saved]-counts[skipped]-counts[missing], pterm.NewStyle(pterm.FgRed)),
	}).Render()

	if code := exitStatus[worst]; code != 0 {
		os.Exit(code)
	}
}

// An empty line means nothing to log (skipped).
func processJob(job Job) (outcome, string) {
	relPath := getRelativePath(job.Exchange, job.Pair, dataType, job.Date)
	fullPath := localPath(relPath)
	label := fmt.Sprintf("%s %s %s", job.Date.Format("2006-01-02"), job.Exchange, job.Pair)

	if _, err := os.Stat(fullPath); err == nil {
		return skipped, ""
	}

	started := time.Now()
	dlURL, size, err := fetchDownloadLink(apiKey, relPath)
	if errors.Is(err, errNotFound) {
		return missing, pterm.Warning.Sprintf("%s  %v", label, err)
	}
	if err == nil {
		err = downloadStream(dlURL, fullPath)
	}
	if err != nil {
		return classify(err), pterm.Error.Sprintf("%s  %v", label, err)
	}
	return saved, pterm.Success.Sprintf("%s  %s  %s", label,
		pterm.Gray(fmt.Sprintf("%.2f MB", float64(size)/1024/1024)),
		pterm.Gray(time.Since(started).Round(100*time.Millisecond)))
}

func classify(err error) outcome {
	switch {
	case errors.Is(err, errAuth):
		return failedAuth
	case errors.Is(err, errUnavailable), errors.Is(err, io.ErrUnexpectedEOF):
		return failedNetwork
	}
	// io.Copy's write errors are *fs.PathError; its read errors are net.Error or io.ErrUnexpectedEOF.
	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return failedDisk
	}
	if _, ok := errors.AsType[*os.LinkError](err); ok {
		return failedDisk
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return failedNetwork
	}
	return failed
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

var (
	errNotFound    = errors.New("not found on server")
	errAPI         = errors.New("api error")
	errAuth        = errors.New("api key rejected")
	errUnavailable = errors.New("server unavailable")
)

var apiURL = "https://7879w58k4l.execute-api.eu-west-1.amazonaws.com/dev/"

func fetchDownloadLink(apiKey, relPath string) (string, int64, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
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

	if resp.StatusCode != http.StatusOK {
		var apiErr APIResponse
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		// The gateway uses "message" for some failures and "error" for others.
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		for _, m := range []string{apiErr.Message, apiErr.Error} {
			if m != "" {
				msg = m
				break
			}
		}
		if resp.StatusCode == http.StatusNotFound {
			// The gateway's usual message just echoes the path we asked for.
			if msg == "File not found: "+relPath {
				return "", 0, errNotFound
			}
			return "", 0, fmt.Errorf("%w: %s", errNotFound, msg)
		}
		kind := errAPI
		switch {
		case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
			kind = errAuth
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
			kind = errUnavailable
		}
		return "", 0, fmt.Errorf("%w: %s", kind, msg)
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

func downloadStream(url, fullPath string) error {
	resp, err := downloadClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", errUnavailable, resp.StatusCode)
	}
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

	if _, err := io.Copy(file, resp.Body); err != nil {
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
