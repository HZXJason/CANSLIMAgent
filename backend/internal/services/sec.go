package services

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SECClient struct {
	enabled     bool
	userAgent   string
	dataBaseURL string
	secBaseURL  string
	client      *http.Client
	minInterval time.Duration

	requestMu sync.Mutex
	lastReq   time.Time

	tickerMu  sync.Mutex
	tickerMap map[string]int
}

type secCompanyFacts struct {
	CIK        int                              `json:"cik"`
	EntityName string                           `json:"entityName"`
	Facts      map[string]map[string]secFactTag `json:"facts"`
}

type secFactTag struct {
	Units map[string][]secFactValue `json:"units"`
}

type secFactValue struct {
	Start string  `json:"start"`
	End   string  `json:"end"`
	Val   float64 `json:"val"`
	FY    int     `json:"fy"`
	FP    string  `json:"fp"`
	Form  string  `json:"form"`
	Filed string  `json:"filed"`
	Frame string  `json:"frame"`
}

type secPeriodValue struct {
	Year    int
	Quarter int
	Date    string
	Filed   string
	Frame   string
	Value   float64
}

type secSubmissions struct {
	CIK     string   `json:"cik"`
	Name    string   `json:"name"`
	Tickers []string `json:"tickers"`
	Filings struct {
		Recent secRecentFilings `json:"recent"`
	} `json:"filings"`
}

type secRecentFilings struct {
	AccessionNumber       []string `json:"accessionNumber"`
	FilingDate            []string `json:"filingDate"`
	ReportDate            []string `json:"reportDate"`
	AcceptanceDateTime    []string `json:"acceptanceDateTime"`
	Form                  []string `json:"form"`
	PrimaryDocument       []string `json:"primaryDocument"`
	PrimaryDocDescription []string `json:"primaryDocDescription"`
	Items                 []string `json:"items"`
}

type SECRecentFiling struct {
	AccessionNumber       string `json:"accession_number"`
	FilingDate            string `json:"filing_date"`
	ReportDate            string `json:"report_date"`
	AcceptanceDateTime    string `json:"acceptance_date_time"`
	Form                  string `json:"form"`
	PrimaryDocument       string `json:"primary_document"`
	PrimaryDocDescription string `json:"primary_doc_description"`
	Items                 string `json:"items"`
	URL                   string `json:"url"`
}

type SECInsiderTransaction struct {
	FilingDate      string   `json:"filing_date"`
	TransactionDate string   `json:"transaction_date"`
	OwnerName       string   `json:"owner_name"`
	OwnerRole       string   `json:"owner_role"`
	SecurityTitle   string   `json:"security_title"`
	Code            string   `json:"code"`
	Action          string   `json:"action"`
	Shares          float64  `json:"shares"`
	Price           *float64 `json:"price"`
	Value           *float64 `json:"value"`
	SharesOwned     *float64 `json:"shares_owned_after"`
	FormURL         string   `json:"form_url"`
}

type secOwnershipDocument struct {
	ReportingOwners []struct {
		ReportingOwnerID struct {
			Name string `xml:"rptOwnerName"`
		} `xml:"reportingOwnerId"`
		Relationship struct {
			IsDirector        string `xml:"isDirector"`
			IsOfficer         string `xml:"isOfficer"`
			IsTenPercentOwner string `xml:"isTenPercentOwner"`
			OfficerTitle      string `xml:"officerTitle"`
		} `xml:"reportingOwnerRelationship"`
	} `xml:"reportingOwner"`
	NonDerivativeTable struct {
		Transactions []struct {
			SecurityTitle string `xml:"securityTitle>value"`
			Date          string `xml:"transactionDate>value"`
			Coding        struct {
				Code string `xml:"transactionCode"`
			} `xml:"transactionCoding"`
			Amounts struct {
				Shares string `xml:"transactionShares>value"`
				Price  string `xml:"transactionPricePerShare>value"`
			} `xml:"transactionAmounts"`
			PostTransaction struct {
				SharesOwned string `xml:"sharesOwnedFollowingTransaction>value"`
			} `xml:"postTransactionAmounts"`
		} `xml:"nonDerivativeTransaction"`
	} `xml:"nonDerivativeTable"`
}

var (
	secEPSTags = []string{
		"EarningsPerShareDiluted",
		"EarningsPerShareBasic",
		"EarningsPerShareDilutedContinuingOperations",
		"EarningsPerShareBasicContinuingOperations",
	}
	secRevenueTags = []string{
		"RevenueFromContractWithCustomerExcludingAssessedTax",
		"Revenues",
		"SalesRevenueNet",
		"SalesRevenueGoodsNet",
	}
	secGrossProfitTags = []string{
		"GrossProfit",
	}
)

func NewSECClient(enabled bool, userAgent string, timeout time.Duration) *SECClient {
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		userAgent = "CANSLIMAgent/1.0 (set SEC_USER_AGENT for contact)"
	}
	return &SECClient{
		enabled:     enabled,
		userAgent:   userAgent,
		dataBaseURL: "https://data.sec.gov",
		secBaseURL:  "https://www.sec.gov",
		client:      &http.Client{Timeout: timeout},
		minInterval: 120 * time.Millisecond,
	}
}

func (c *SECClient) Configured() bool {
	return c != nil && c.enabled
}

func (c *SECClient) FetchEPSGrowth(ctx context.Context, symbol string) (quarterGrowth *float64, quarterSufficient bool, annualGrowth []float64, yearsAvailable int, annualSufficient bool, dataAsOf time.Time, err error) {
	if !c.Configured() {
		return nil, false, nil, 0, false, time.Time{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return nil, false, nil, 0, false, time.Time{}, err
	}

	epsData := buildSECEPSQuarterly(symbol, facts)
	if len(epsData.Quarters) > 0 {
		latest := epsData.Quarters[0]
		value := latest.EPSYoYGrowth
		if !latest.DataSufficient {
			value = latest.EPS
		}
		quarterGrowth = &value
		quarterSufficient = latest.DataSufficient
		if parsed, parseErr := time.Parse("2006-01-02", latest.ReportDate); parseErr == nil {
			dataAsOf = parsed
		}
	}

	annualGrowth, annualSufficient = buildSECAnnualEPSGrowth(facts)
	return quarterGrowth, quarterGrowth != nil && quarterSufficient, annualGrowth, len(annualGrowth), annualSufficient, dataAsOf, nil
}

func (c *SECClient) FetchEPSQuarterly(ctx context.Context, symbol string) (EPSData, error) {
	if !c.Configured() {
		return EPSData{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return EPSData{}, err
	}
	data := buildSECEPSQuarterly(symbol, facts)
	if len(data.Quarters) == 0 {
		return data, errors.New("SEC EDGAR EPS data is empty")
	}
	return data, nil
}

func (c *SECClient) FetchRevenueAnnual(ctx context.Context, symbol string) (RevenueData, error) {
	if !c.Configured() {
		return RevenueData{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return RevenueData{}, err
	}
	data := buildSECRevenueAnnual(symbol, facts)
	if len(data.Years) == 0 {
		return data, errors.New("SEC EDGAR annual revenue data is empty")
	}
	return data, nil
}

func (c *SECClient) FetchGrossMarginQuarterly(ctx context.Context, symbol string) (GrossMarginData, error) {
	if !c.Configured() {
		return GrossMarginData{}, errors.New("SEC EDGAR 数据源未启用")
	}
	facts, err := c.fetchCompanyFacts(ctx, symbol)
	if err != nil {
		return GrossMarginData{}, err
	}
	data := buildSECGrossMarginQuarterly(symbol, facts)
	if len(data.Quarters) == 0 {
		return data, errors.New("SEC EDGAR gross margin data is empty")
	}
	return data, nil
}

func (c *SECClient) FetchSecondLayerSignals(ctx context.Context, symbol string) (map[string]interface{}, error) {
	if !c.Configured() {
		return nil, errors.New("SEC EDGAR 数据源未启用")
	}
	cik, err := c.lookupCIK(ctx, symbol)
	if err != nil {
		return nil, err
	}
	submissions, err := c.fetchSubmissions(ctx, cik)
	if err != nil {
		return nil, err
	}

	recentFilings := buildSECRecentFilings(cik, submissions.Filings.Recent, 80)
	eventFilings := filterSECEventFilings(recentFilings, 30)
	insiderTransactions := c.fetchRecentForm4Transactions(ctx, cik, recentFilings, 20)

	return map[string]interface{}{
		"source":               "SEC EDGAR",
		"cik":                  cik,
		"company_name":         submissions.Name,
		"tickers":              submissions.Tickers,
		"recent_event_filings": eventFilings,
		"insider_transactions": insiderTransactions,
		"insider_summary":      summarizeSECInsiderTransactions(insiderTransactions),
	}, nil
}

func (c *SECClient) fetchCompanyFacts(ctx context.Context, symbol string) (secCompanyFacts, error) {
	cik, err := c.lookupCIK(ctx, symbol)
	if err != nil {
		return secCompanyFacts{}, err
	}

	var payload secCompanyFacts
	path := fmt.Sprintf("/api/xbrl/companyfacts/CIK%010d.json", cik)
	if err := c.getJSON(ctx, c.dataBaseURL+path, &payload); err != nil {
		return secCompanyFacts{}, err
	}
	if payload.CIK == 0 {
		payload.CIK = cik
	}
	return payload, nil
}

func (c *SECClient) fetchSubmissions(ctx context.Context, cik int) (secSubmissions, error) {
	var payload secSubmissions
	path := fmt.Sprintf("/submissions/CIK%010d.json", cik)
	if err := c.getJSON(ctx, c.dataBaseURL+path, &payload); err != nil {
		return secSubmissions{}, err
	}
	return payload, nil
}

func (c *SECClient) lookupCIK(ctx context.Context, symbol string) (int, error) {
	symbol = normalizeSECTicker(symbol)
	if symbol == "" {
		return 0, errors.New("股票代码不能为空")
	}

	c.tickerMu.Lock()
	defer c.tickerMu.Unlock()

	if c.tickerMap == nil {
		var rows map[string]struct {
			CIKStr int    `json:"cik_str"`
			Ticker string `json:"ticker"`
			Title  string `json:"title"`
		}
		if err := c.getJSON(ctx, c.secBaseURL+"/files/company_tickers.json", &rows); err != nil {
			return 0, err
		}
		c.tickerMap = make(map[string]int, len(rows))
		for _, row := range rows {
			ticker := normalizeSECTicker(row.Ticker)
			if ticker != "" && row.CIKStr > 0 {
				c.tickerMap[ticker] = row.CIKStr
			}
		}
	}
	if cik, ok := c.tickerMap[symbol]; ok {
		return cik, nil
	}
	return 0, fmt.Errorf("SEC EDGAR 未找到 %s 对应的 CIK", symbol)
}

func (c *SECClient) getJSON(ctx context.Context, endpoint string, target interface{}) error {
	if err := c.waitForSECSlot(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("SEC EDGAR 请求失败：HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 100<<20)).Decode(target)
}

func (c *SECClient) getXML(ctx context.Context, endpoint string, target interface{}) error {
	if err := c.waitForSECSlot(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/xml,text/xml,*/*")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("SEC EDGAR XML 请求失败：HTTP %d", resp.StatusCode)
	}
	return xml.NewDecoder(io.LimitReader(resp.Body, 50<<20)).Decode(target)
}

func buildSECRecentFilings(cik int, recent secRecentFilings, limit int) []SECRecentFiling {
	count := len(recent.AccessionNumber)
	if limit > 0 && count > limit {
		count = limit
	}
	filings := make([]SECRecentFiling, 0, count)
	for index := 0; index < count; index++ {
		accession := secRecentValue(recent.AccessionNumber, index)
		if accession == "" {
			continue
		}
		primaryDocument := secRecentValue(recent.PrimaryDocument, index)
		filings = append(filings, SECRecentFiling{
			AccessionNumber:       accession,
			FilingDate:            secRecentValue(recent.FilingDate, index),
			ReportDate:            secRecentValue(recent.ReportDate, index),
			AcceptanceDateTime:    secRecentValue(recent.AcceptanceDateTime, index),
			Form:                  secRecentValue(recent.Form, index),
			PrimaryDocument:       primaryDocument,
			PrimaryDocDescription: secRecentValue(recent.PrimaryDocDescription, index),
			Items:                 secRecentValue(recent.Items, index),
			URL:                   secArchiveDocumentURL(cik, accession, primaryDocument),
		})
	}
	return filings
}

func filterSECEventFilings(filings []SECRecentFiling, limit int) []SECRecentFiling {
	out := make([]SECRecentFiling, 0, min(limit, len(filings)))
	for _, filing := range filings {
		form := strings.ToUpper(strings.TrimSpace(filing.Form))
		if form == "4" || form == "4/A" || form == "8-K" || form == "10-Q" || form == "10-K" ||
			form == "S-1" || form == "S-1/A" || form == "S-3" || form == "S-3/A" ||
			strings.HasPrefix(form, "424B") || form == "DEF 14A" {
			out = append(out, filing)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

func (c *SECClient) fetchRecentForm4Transactions(ctx context.Context, cik int, filings []SECRecentFiling, filingLimit int) []SECInsiderTransaction {
	transactions := []SECInsiderTransaction{}
	checked := 0
	for _, filing := range filings {
		form := strings.ToUpper(strings.TrimSpace(filing.Form))
		if form != "4" && form != "4/A" {
			continue
		}
		if filing.URL == "" {
			continue
		}
		var doc secOwnershipDocument
		if err := c.getXML(ctx, filing.URL, &doc); err != nil {
			continue
		}
		transactions = append(transactions, ownershipTransactions(filing, doc)...)
		checked++
		if filingLimit > 0 && checked >= filingLimit {
			break
		}
	}
	return transactions
}

func ownershipTransactions(filing SECRecentFiling, doc secOwnershipDocument) []SECInsiderTransaction {
	ownerName, ownerRole := secOwnershipOwner(doc)
	out := make([]SECInsiderTransaction, 0, len(doc.NonDerivativeTable.Transactions))
	for _, row := range doc.NonDerivativeTable.Transactions {
		shares, ok := parseSECXMLFloat(row.Amounts.Shares)
		if !ok || shares == 0 {
			continue
		}
		price, hasPrice := parseSECXMLFloat(row.Amounts.Price)
		value := 0.0
		hasValue := hasPrice
		if hasValue {
			value = round2(shares * price)
		}
		sharesOwned, hasSharesOwned := parseSECXMLFloat(row.PostTransaction.SharesOwned)
		transaction := SECInsiderTransaction{
			FilingDate:      filing.FilingDate,
			TransactionDate: secXMLText(row.Date),
			OwnerName:       ownerName,
			OwnerRole:       ownerRole,
			SecurityTitle:   secXMLText(row.SecurityTitle),
			Code:            strings.ToUpper(strings.TrimSpace(row.Coding.Code)),
			Action:          secTransactionAction(row.Coding.Code),
			Shares:          round2(shares),
			FormURL:         filing.URL,
		}
		if hasPrice {
			transaction.Price = &price
		}
		if hasValue {
			transaction.Value = &value
		}
		if hasSharesOwned {
			transaction.SharesOwned = &sharesOwned
		}
		out = append(out, transaction)
	}
	return out
}

func summarizeSECInsiderTransactions(transactions []SECInsiderTransaction) map[string]interface{} {
	summary := map[string]interface{}{
		"transaction_count": len(transactions),
	}
	purchaseCount, saleCount := 0, 0
	purchaseShares, saleShares := 0.0, 0.0
	purchaseValue, saleValue := 0.0, 0.0
	for _, transaction := range transactions {
		switch transaction.Action {
		case "purchase":
			purchaseCount++
			purchaseShares += transaction.Shares
			if transaction.Value != nil {
				purchaseValue += *transaction.Value
			}
		case "sale":
			saleCount++
			saleShares += transaction.Shares
			if transaction.Value != nil {
				saleValue += *transaction.Value
			}
		}
	}
	summary["purchase_count"] = purchaseCount
	summary["sale_count"] = saleCount
	summary["purchase_shares"] = round2(purchaseShares)
	summary["sale_shares"] = round2(saleShares)
	summary["purchase_value"] = round2(purchaseValue)
	summary["sale_value"] = round2(saleValue)
	if len(transactions) > 0 {
		summary["latest_transaction_date"] = transactions[0].TransactionDate
	}
	return summary
}

func secRecentValue(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[index])
}

func secArchiveDocumentURL(cik int, accession string, primaryDocument string) string {
	accession = strings.ReplaceAll(strings.TrimSpace(accession), "-", "")
	primaryDocument = strings.TrimSpace(primaryDocument)
	if strings.HasPrefix(strings.ToLower(primaryDocument), "xsl") {
		if parts := strings.Split(primaryDocument, "/"); len(parts) > 0 {
			primaryDocument = parts[len(parts)-1]
		}
	}
	if cik <= 0 || accession == "" || primaryDocument == "" {
		return ""
	}
	return fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%d/%s/%s", cik, accession, primaryDocument)
}

func secOwnershipOwner(doc secOwnershipDocument) (string, string) {
	if len(doc.ReportingOwners) == 0 {
		return "", ""
	}
	owner := doc.ReportingOwners[0]
	roles := []string{}
	if secXMLBool(owner.Relationship.IsDirector) {
		roles = append(roles, "director")
	}
	if secXMLBool(owner.Relationship.IsOfficer) {
		title := secXMLText(owner.Relationship.OfficerTitle)
		if title != "" {
			roles = append(roles, title)
		} else {
			roles = append(roles, "officer")
		}
	}
	if secXMLBool(owner.Relationship.IsTenPercentOwner) {
		roles = append(roles, "10% owner")
	}
	return secXMLText(owner.ReportingOwnerID.Name), strings.Join(roles, ", ")
}

func secXMLBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

func secXMLText(value string) string {
	return strings.TrimSpace(value)
}

func parseSECXMLFloat(value string) (float64, bool) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

func secTransactionAction(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "P":
		return "purchase"
	case "S":
		return "sale"
	case "A":
		return "grant"
	case "D":
		return "disposition"
	case "F":
		return "tax_withholding"
	case "M":
		return "option_exercise"
	default:
		return "other"
	}
}

func (c *SECClient) waitForSECSlot(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}
	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	if !c.lastReq.IsZero() {
		wait := c.minInterval - time.Since(c.lastReq)
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	c.lastReq = time.Now()
	return nil
}

func buildSECEPSQuarterly(symbol string, facts secCompanyFacts) EPSData {
	data := EPSData{Symbol: normalizeSymbol(symbol), Source: "SEC EDGAR", Quarters: []EPSQuarter{}}
	values := selectSECPeriodValues(facts, secEPSTags, []string{"USD/shares"}, true)
	limit := min(12, len(values))
	byPeriod := make(map[string]secPeriodValue, len(values))
	for _, value := range values {
		byPeriod[secQuarterKey(value.Year, value.Quarter)] = value
	}

	for i := 0; i < limit; i++ {
		value := values[i]
		growth := round2(value.Value)
		sufficient := false
		if previous, ok := byPeriod[secQuarterKey(value.Year-1, value.Quarter)]; ok && previous.Value != 0 {
			growth = round2((value.Value - previous.Value) / math.Abs(previous.Value) * 100)
			sufficient = true
		}
		data.Quarters = append(data.Quarters, EPSQuarter{
			Period:         fmt.Sprintf("%dQ%d", value.Year, value.Quarter),
			EPS:            round2(value.Value),
			EPSYoYGrowth:   growth,
			DataSufficient: sufficient,
			ReportDate:     value.Date,
		})
	}
	if len(data.Quarters) > 0 {
		data.DataAsOf = data.Quarters[0].ReportDate
	}
	return data
}

func buildSECRevenueAnnual(symbol string, facts secCompanyFacts) RevenueData {
	data := RevenueData{Symbol: normalizeSymbol(symbol), Source: "SEC EDGAR", Years: []RevenueYear{}}
	values := selectSECPeriodValues(facts, secRevenueTags, []string{"USD"}, false)
	limit := min(5, len(values))
	byYear := make(map[int]secPeriodValue, len(values))
	for _, value := range values {
		byYear[value.Year] = value
	}

	for i := 0; i < limit; i++ {
		value := values[i]
		growth := 0.0
		sufficient := false
		if previous, ok := byYear[value.Year-1]; ok && previous.Value != 0 {
			growth = round2((value.Value - previous.Value) / math.Abs(previous.Value) * 100)
			sufficient = true
		}
		data.Years = append(data.Years, RevenueYear{
			Year:             value.Year,
			Revenue:          value.Value,
			RevenueYoYGrowth: growth,
			DataSufficient:   sufficient,
		})
	}
	if len(values) > 0 {
		data.DataAsOf = values[0].Date
	}
	return data
}

func buildSECGrossMarginQuarterly(symbol string, facts secCompanyFacts) GrossMarginData {
	data := GrossMarginData{Symbol: normalizeSymbol(symbol), Source: "SEC EDGAR", Quarters: []GrossMarginQuarter{}}
	revenue := selectSECPeriodValues(facts, secRevenueTags, []string{"USD"}, true)
	grossProfit := selectSECPeriodValues(facts, secGrossProfitTags, []string{"USD"}, true)
	grossByPeriod := make(map[string]secPeriodValue, len(grossProfit))
	for _, value := range grossProfit {
		grossByPeriod[secQuarterKey(value.Year, value.Quarter)] = value
	}

	for _, revenueValue := range revenue {
		if len(data.Quarters) >= 12 {
			break
		}
		if revenueValue.Value == 0 {
			continue
		}
		grossValue, ok := grossByPeriod[secQuarterKey(revenueValue.Year, revenueValue.Quarter)]
		if !ok {
			continue
		}
		data.Quarters = append(data.Quarters, GrossMarginQuarter{
			Period:      fmt.Sprintf("%dQ%d", revenueValue.Year, revenueValue.Quarter),
			GrossMargin: round2(grossValue.Value / revenueValue.Value * 100),
			ReportDate:  revenueValue.Date,
		})
	}
	if len(data.Quarters) > 0 {
		data.DataAsOf = data.Quarters[0].ReportDate
	}
	return data
}

func buildSECAnnualEPSGrowth(facts secCompanyFacts) ([]float64, bool) {
	values := selectSECPeriodValues(facts, secEPSTags, []string{"USD/shares"}, false)
	byYear := make(map[int]secPeriodValue, len(values))
	for _, value := range values {
		byYear[value.Year] = value
	}

	growth := make([]float64, 0, 3)
	sufficient := len(values) >= 4
	for i := 0; i < len(values) && len(growth) < 3; i++ {
		current := values[i]
		previous, ok := byYear[current.Year-1]
		if !ok || previous.Value == 0 {
			sufficient = false
			growth = append(growth, round2(current.Value))
			continue
		}
		growth = append(growth, round2((current.Value-previous.Value)/math.Abs(previous.Value)*100))
	}
	if len(growth) < 3 {
		sufficient = false
	}
	return growth, sufficient && len(growth) >= 3
}

func selectSECPeriodValues(facts secCompanyFacts, tags []string, units []string, quarterly bool) []secPeriodValue {
	raw := secFactValues(facts, tags, units)
	selected := make(map[string]secPeriodValue)
	for _, row := range raw {
		value, ok := secValueFromFrame(row, quarterly)
		if !ok {
			continue
		}
		replaceSECPeriodValue(selected, value)
	}
	if len(selected) < secMinimumFrameCount(quarterly) {
		for _, row := range raw {
			value, ok := secValueFromDuration(row, quarterly)
			if !ok {
				continue
			}
			replaceSECPeriodValue(selected, value)
		}
	}

	values := make([]secPeriodValue, 0, len(selected))
	for _, value := range selected {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Date == values[j].Date {
			return values[i].Filed > values[j].Filed
		}
		return values[i].Date > values[j].Date
	})
	return values
}

func secFactValues(facts secCompanyFacts, tags []string, units []string) []secFactValue {
	usGAAP := facts.Facts["us-gaap"]
	out := []secFactValue{}
	for _, tagName := range tags {
		tag, ok := usGAAP[tagName]
		if !ok {
			continue
		}
		if rows, ok := selectSECUnitRows(tag, units); ok {
			out = append(out, rows...)
		}
	}
	return out
}

func selectSECUnitRows(tag secFactTag, units []string) ([]secFactValue, bool) {
	for _, unit := range units {
		if rows, ok := tag.Units[unit]; ok {
			return rows, true
		}
	}
	for unitName, rows := range tag.Units {
		for _, unit := range units {
			if strings.EqualFold(unitName, unit) || strings.Contains(strings.ToLower(unitName), strings.ToLower(unit)) {
				return rows, true
			}
		}
	}
	for _, rows := range tag.Units {
		return rows, true
	}
	return nil, false
}

func secValueFromFrame(row secFactValue, quarterly bool) (secPeriodValue, bool) {
	if !secAcceptedForm(row.Form) || row.End == "" || row.Frame == "" {
		return secPeriodValue{}, false
	}
	year, quarter, ok := parseSECFrame(row.Frame, quarterly)
	if !ok {
		return secPeriodValue{}, false
	}
	return secPeriodValue{
		Year:    year,
		Quarter: quarter,
		Date:    row.End,
		Filed:   row.Filed,
		Frame:   row.Frame,
		Value:   row.Val,
	}, true
}

func secValueFromDuration(row secFactValue, quarterly bool) (secPeriodValue, bool) {
	if !secAcceptedForm(row.Form) || row.Start == "" || row.End == "" {
		return secPeriodValue{}, false
	}
	start, err := time.Parse("2006-01-02", row.Start)
	if err != nil {
		return secPeriodValue{}, false
	}
	end, err := time.Parse("2006-01-02", row.End)
	if err != nil {
		return secPeriodValue{}, false
	}
	days := int(end.Sub(start).Hours()/24) + 1
	year := row.FY
	if year == 0 {
		year = end.Year()
	}
	quarter := 0
	if quarterly {
		if days < 60 || days > 120 {
			return secPeriodValue{}, false
		}
		quarter = int((end.Month()-1)/3) + 1
	} else {
		if days < 300 || days > 380 {
			return secPeriodValue{}, false
		}
	}
	return secPeriodValue{
		Year:    year,
		Quarter: quarter,
		Date:    row.End,
		Filed:   row.Filed,
		Value:   row.Val,
	}, true
}

func replaceSECPeriodValue(values map[string]secPeriodValue, next secPeriodValue) {
	key := strconv.Itoa(next.Year)
	if next.Quarter > 0 {
		key = secQuarterKey(next.Year, next.Quarter)
	}
	current, ok := values[key]
	if !ok || next.Filed > current.Filed || (next.Filed == current.Filed && next.Date > current.Date) {
		values[key] = next
	}
}

func parseSECFrame(frame string, quarterly bool) (int, int, bool) {
	frame = strings.ToUpper(strings.TrimSpace(frame))
	if !strings.HasPrefix(frame, "CY") || len(frame) < 6 {
		return 0, 0, false
	}
	year, err := strconv.Atoi(frame[2:6])
	if err != nil {
		return 0, 0, false
	}
	if quarterly {
		if len(frame) != 8 || frame[6] != 'Q' {
			return 0, 0, false
		}
		quarter := int(frame[7] - '0')
		if quarter < 1 || quarter > 4 {
			return 0, 0, false
		}
		return year, quarter, true
	}
	if len(frame) == 6 {
		return year, 0, true
	}
	return 0, 0, false
}

func secAcceptedForm(form string) bool {
	switch strings.ToUpper(strings.TrimSpace(form)) {
	case "10-K", "10-K/A", "10-Q", "10-Q/A", "20-F", "20-F/A", "40-F", "40-F/A":
		return true
	default:
		return false
	}
}

func secMinimumFrameCount(quarterly bool) int {
	if quarterly {
		return 5
	}
	return 4
}

func secQuarterKey(year int, quarter int) string {
	return fmt.Sprintf("%04dQ%d", year, quarter)
}

func normalizeSECTicker(value string) string {
	return strings.ToUpper(strings.NewReplacer(".", "-", "/", "-").Replace(strings.TrimSpace(value)))
}
