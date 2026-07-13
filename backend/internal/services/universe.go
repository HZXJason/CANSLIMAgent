package services

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/alphalens/canslimagent/backend/internal/models"
)

type FreeUniverseClient struct {
	enabled bool
	client  *http.Client

	sp500URLs       []string
	nasdaq100URL    string
	russell2000URL  string
	nasdaqListedURL string
	otherListedURL  string
}

func NewFreeUniverseClient(enabled bool, timeout time.Duration) *FreeUniverseClient {
	return &FreeUniverseClient{
		enabled: enabled,
		client:  &http.Client{Timeout: timeout},
		sp500URLs: []string{
			"https://cdn.jsdelivr.net/gh/datasets/s-and-p-500-companies@master/data/constituents.csv",
			"https://raw.githubusercontent.com/datasets/s-and-p-500-companies/master/data/constituents.csv",
		},
		nasdaq100URL:    "https://api.nasdaq.com/api/quote/list-type/nasdaq100",
		russell2000URL:  "https://www.blackrock.com/varnish-api/blk-one01-product-data/product-data/api/v1/get-fund-document?appSubType=ISHARES&appType=PRODUCT_PAGE&component=fundDownload&locale=en_US&portfolioId=239710&targetSite=us-ishares&userType=individual",
		nasdaqListedURL: "https://www.nasdaqtrader.com/dynamic/SymDir/nasdaqlisted.txt",
		otherListedURL:  "https://www.nasdaqtrader.com/dynamic/SymDir/otherlisted.txt",
	}
}

func (c *FreeUniverseClient) Configured() bool {
	return c != nil && c.enabled
}

func (c *FreeUniverseClient) FetchUniverse(ctx context.Context, market string) ([]StockCandidate, error) {
	if !c.Configured() {
		return nil, errors.New("免费股票池数据源未启用")
	}

	var candidates []StockCandidate
	var err error
	switch market {
	case models.MarketSP500:
		candidates, err = c.fetchSP500(ctx)
	case models.MarketNASDAQ100:
		candidates, err = c.fetchNASDAQ100(ctx)
	case models.MarketRUSSELL2000:
		candidates, err = c.fetchRussell2000(ctx)
	default:
		return nil, fmt.Errorf("%s 暂未配置免费成分股来源", market)
	}
	if err != nil {
		return nil, err
	}
	candidates = normalizeCandidates(candidates)
	if len(candidates) == 0 {
		return nil, errors.New("免费股票池返回为空")
	}
	candidates = c.enrichWithNasdaqTrader(ctx, candidates)
	return normalizeCandidates(candidates), nil
}

func (c *FreeUniverseClient) fetchSP500(ctx context.Context) ([]StockCandidate, error) {
	var lastErr error
	for _, endpoint := range c.sp500URLs {
		candidates, err := c.fetchSP500FromURL(ctx, endpoint)
		if err == nil && len(candidates) > 0 {
			return candidates, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("SP500 CSV 返回为空")
}

func (c *FreeUniverseClient) fetchSP500FromURL(ctx context.Context, endpoint string) ([]StockCandidate, error) {
	resp, err := c.get(ctx, endpoint, "text/csv,*/*")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	header, err := reader.Read()
	if err != nil {
		return nil, err
	}
	symbolIndex := csvColumnIndex(header, "Symbol")
	nameIndex := csvColumnIndex(header, "Security")
	if symbolIndex < 0 || nameIndex < 0 {
		return nil, errors.New("SP500 CSV 缺少 Symbol/Security 列")
	}

	candidates := []StockCandidate{}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(row) <= max(symbolIndex, nameIndex) {
			continue
		}
		candidates = append(candidates, StockCandidate{
			Symbol: normalizeUniverseSymbol(row[symbolIndex]),
			Name:   strings.TrimSpace(row[nameIndex]),
		})
	}
	return candidates, nil
}

func (c *FreeUniverseClient) fetchNASDAQ100(ctx context.Context) ([]StockCandidate, error) {
	resp, err := c.get(ctx, c.nasdaq100URL, "application/json,*/*")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		Data struct {
			Data struct {
				Rows []struct {
					Symbol      string `json:"symbol"`
					CompanyName string `json:"companyName"`
				} `json:"rows"`
			} `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	candidates := make([]StockCandidate, 0, len(payload.Data.Data.Rows))
	for _, row := range payload.Data.Data.Rows {
		candidates = append(candidates, StockCandidate{
			Symbol: normalizeUniverseSymbol(row.Symbol),
			Name:   cleanSecurityName(row.CompanyName),
		})
	}
	return candidates, nil
}

func (c *FreeUniverseClient) fetchRussell2000(ctx context.Context) ([]StockCandidate, error) {
	resp, err := c.get(ctx, c.russell2000URL, "application/vnd.ms-excel,application/xml,text/xml,*/*")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseIWMHoldingsSpreadsheet(resp.Body)
}

func (c *FreeUniverseClient) enrichWithNasdaqTrader(ctx context.Context, candidates []StockCandidate) []StockCandidate {
	listed, err := c.fetchNasdaqTraderNames(ctx)
	if err != nil || len(listed) == 0 {
		return candidates
	}
	return enrichCandidatesWithListings(candidates, listed)
}

func enrichCandidatesWithListings(candidates []StockCandidate, listed map[string]string) []StockCandidate {
	nameIndex := buildNasdaqTraderNameIndex(listed)
	out := make([]StockCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		symbol := normalizeUniverseSymbol(candidate.Symbol)
		if symbol == "" {
			continue
		}
		if name, ok := listed[symbol]; ok && strings.TrimSpace(candidate.Name) == "" {
			candidate.Name = name
		} else if _, ok := listed[symbol]; !ok {
			if remapped := nameIndex[canonicalSecurityName(candidate.Name)]; remapped != "" {
				symbol = remapped
				candidate.Symbol = remapped
				if strings.TrimSpace(candidate.Name) == "" {
					candidate.Name = listed[remapped]
				}
			}
		}
		candidate.Symbol = symbol
		out = append(out, candidate)
	}
	return out
}

func (c *FreeUniverseClient) fetchNasdaqTraderNames(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if err := c.fetchPipeDirectory(ctx, c.nasdaqListedURL, "Symbol", "Security Name", out); err != nil {
		return nil, err
	}
	if err := c.fetchPipeDirectory(ctx, c.otherListedURL, "ACT Symbol", "Security Name", out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *FreeUniverseClient) fetchPipeDirectory(ctx context.Context, endpoint string, symbolColumn string, nameColumn string, out map[string]string) error {
	resp, err := c.get(ctx, endpoint, "text/plain,*/*")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.Comma = '|'
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return err
	}
	symbolIndex := csvColumnIndex(header, symbolColumn)
	nameIndex := csvColumnIndex(header, nameColumn)
	etfIndex := csvColumnIndex(header, "ETF")
	testIndex := csvColumnIndex(header, "Test Issue")
	if symbolIndex < 0 || nameIndex < 0 {
		return fmt.Errorf("Nasdaq Trader 目录缺少 %s/%s 列", symbolColumn, nameColumn)
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(row) <= max(symbolIndex, nameIndex) {
			continue
		}
		if etfIndex >= 0 && len(row) > etfIndex && strings.EqualFold(strings.TrimSpace(row[etfIndex]), "Y") {
			continue
		}
		if testIndex >= 0 && len(row) > testIndex && strings.EqualFold(strings.TrimSpace(row[testIndex]), "Y") {
			continue
		}
		symbol := normalizeUniverseSymbol(row[symbolIndex])
		name := cleanSecurityName(row[nameIndex])
		if symbol != "" && name != "" {
			out[symbol] = name
		}
	}
	return nil
}

func (c *FreeUniverseClient) get(ctx context.Context, endpoint string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 CANSLIMAgent/1.0")
	req.Header.Set("Accept", accept)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("免费股票池响应关闭失败：%w", closeErr)
		}
		return nil, fmt.Errorf("免费股票池请求失败：HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

type spreadsheetRow struct {
	Cells []spreadsheetCell `xml:"Cell"`
}

type spreadsheetCell struct {
	Index int    `xml:"Index,attr"`
	Data  string `xml:"Data"`
}

func parseIWMHoldingsSpreadsheet(reader io.Reader) ([]StockCandidate, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	worksheet, err := extractSpreadsheetWorksheet(body, "Holdings")
	if err != nil {
		return nil, err
	}

	decoder := xml.NewDecoder(strings.NewReader(worksheet))
	tickerIndex := -1
	nameIndex := -1
	assetClassIndex := -1
	candidates := []StockCandidate{}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析 IWM 持仓文件失败：%w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Row" {
			continue
		}

		var row spreadsheetRow
		if err := decoder.DecodeElement(&row, &start); err != nil {
			return nil, fmt.Errorf("解析 IWM 持仓行失败：%w", err)
		}
		values := spreadsheetRowValues(row)
		if tickerIndex < 0 {
			tickerIndex = csvColumnIndex(values, "Ticker")
			nameIndex = csvColumnIndex(values, "Name")
			assetClassIndex = csvColumnIndex(values, "Asset Class")
			if tickerIndex < 0 || nameIndex < 0 || assetClassIndex < 0 {
				tickerIndex = -1
				nameIndex = -1
				assetClassIndex = -1
			}
			continue
		}
		if len(values) <= max(tickerIndex, nameIndex, assetClassIndex) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(values[assetClassIndex]), "Equity") {
			continue
		}
		symbol := normalizeUniverseSymbol(values[tickerIndex])
		name := cleanSecurityName(values[nameIndex])
		if symbol == "" || name == "" {
			continue
		}
		candidates = append(candidates, StockCandidate{Symbol: symbol, Name: name})
	}

	if tickerIndex < 0 {
		return nil, errors.New("IWM 持仓文件缺少 Holdings/Ticker/Name/Asset Class 表头")
	}
	if len(candidates) == 0 {
		return nil, errors.New("IWM 持仓文件没有可用股票行")
	}
	return candidates, nil
}

func extractSpreadsheetWorksheet(body []byte, name string) (string, error) {
	text := string(body)
	nameMarker := fmt.Sprintf(`ss:Name="%s"`, name)
	nameIndex := strings.Index(text, nameMarker)
	if nameIndex < 0 {
		return "", fmt.Errorf("IWM 持仓文件缺少 %s 工作表", name)
	}
	start := strings.LastIndex(text[:nameIndex], "<ss:Worksheet")
	if start < 0 {
		return "", fmt.Errorf("IWM 持仓文件缺少 %s 工作表起始标签", name)
	}
	endMarker := "</ss:Worksheet>"
	endOffset := strings.Index(text[nameIndex:], endMarker)
	if endOffset < 0 {
		return "", fmt.Errorf("IWM 持仓文件缺少 %s 工作表结束标签", name)
	}
	end := nameIndex + endOffset + len(endMarker)
	return `<ss:Workbook xmlns:ss="urn:schemas-microsoft-com:office:spreadsheet">` + text[start:end] + `</ss:Workbook>`, nil
}

func spreadsheetRowValues(row spreadsheetRow) []string {
	values := []string{}
	for _, cell := range row.Cells {
		if cell.Index > 0 {
			for len(values) < cell.Index-1 {
				values = append(values, "")
			}
		}
		values = append(values, strings.TrimSpace(cell.Data))
	}
	return values
}

func buildNasdaqTraderNameIndex(listed map[string]string) map[string]string {
	index := make(map[string]string, len(listed))
	for symbol, name := range listed {
		key := canonicalSecurityName(name)
		if key == "" {
			continue
		}
		if existing, ok := index[key]; ok && existing != symbol {
			index[key] = ""
			continue
		}
		index[key] = symbol
	}
	return index
}

func canonicalSecurityName(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"&", " AND ",
		".", " ",
		",", " ",
		"'", "",
		"\"", "",
		"(", " ",
		")", " ",
		"/", " ",
		"-", " ",
	).Replace(value)
	for _, suffix := range []string{
		" COMMON STOCK",
		" COMMON SHARES",
		" ORDINARY SHARES",
		" ORDINARY SHARE",
		" SHARES",
		" SHARE",
	} {
		value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
	}
	return strings.Join(strings.Fields(value), " ")
}

func normalizeCandidates(candidates []StockCandidate) []StockCandidate {
	seen := make(map[string]StockCandidate, len(candidates))
	for _, candidate := range candidates {
		symbol := normalizeUniverseSymbol(candidate.Symbol)
		name := strings.TrimSpace(candidate.Name)
		if symbol == "" || name == "" {
			continue
		}
		seen[symbol] = StockCandidate{Symbol: symbol, Name: name}
	}
	out := make([]StockCandidate, 0, len(seen))
	for _, candidate := range seen {
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Symbol < out[j].Symbol
	})
	return out
}

func csvColumnIndex(header []string, name string) int {
	for index, column := range header {
		if strings.EqualFold(strings.TrimSpace(column), name) {
			return index
		}
	}
	return -1
}

func normalizeUniverseSymbol(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	value = strings.ToUpper(strings.NewReplacer(".", "-", "/", "-").Replace(value))
	if value == "" || strings.ContainsAny(value, " ^$") {
		return ""
	}
	hasLetter := false
	for _, character := range value {
		if character >= 'A' && character <= 'Z' {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return ""
	}
	return value
}

func cleanSecurityName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, " Common Stock")
	value = strings.TrimSuffix(value, " Class A Common Stock")
	value = strings.TrimSuffix(value, " Class B Common Stock")
	return strings.TrimSpace(value)
}
