package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/saucesteals/monitord"
)

var nvdAPIKey = monitord.OptionalSecret("nvd", "api-key")
var cveIDPattern = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)

const nvdEndpoint = "https://services.nvd.nist.gov/rest/json/cves/2.0"

// CVE is the subset of the NVD response used for matching and notifications.
type CVE struct {
	ID           string `json:"id"`
	Published    string `json:"published"`
	LastModified string `json:"lastModified"`
	Status       string `json:"vulnStatus"`
	KEVAdded     string `json:"cisaExploitAdd"`
	Descriptions []struct {
		Lang  string `json:"lang"`
		Value string `json:"value"`
	} `json:"descriptions"`
	Metrics struct {
		V40 []Metric `json:"cvssMetricV40"`
		V31 []Metric `json:"cvssMetricV31"`
		V30 []Metric `json:"cvssMetricV30"`
		V2  []Metric `json:"cvssMetricV2"`
	} `json:"metrics"`
	Configurations []struct {
		Nodes []struct {
			Matches []struct {
				Vulnerable     bool   `json:"vulnerable"`
				Criteria       string `json:"criteria"`
				StartIncluding string `json:"versionStartIncluding"`
				StartExcluding string `json:"versionStartExcluding"`
				EndIncluding   string `json:"versionEndIncluding"`
				EndExcluding   string `json:"versionEndExcluding"`
			} `json:"cpeMatch"`
		} `json:"nodes"`
	} `json:"configurations"`
	Affected []struct {
		Data []struct {
			Vendor  string `json:"vendor"`
			Product string `json:"product"`
		} `json:"affectedData"`
	} `json:"affected"`
	References []struct {
		URL string `json:"url"`
	} `json:"references"`
}

// Metric retains the version and source of a CVSS base score.
type Metric struct {
	Source string `json:"source"`
	Data   struct {
		Version string   `json:"version"`
		Score   *float64 `json:"baseScore"`
		Vector  string   `json:"vectorString"`
	} `json:"cvssData"`
}

type page struct {
	StartIndex      int    `json:"startIndex"`
	TotalResults    int    `json:"totalResults"`
	ResultsPerPage  int    `json:"resultsPerPage"`
	Format          string `json:"format"`
	Version         string `json:"version"`
	Vulnerabilities []struct {
		CVE CVE `json:"cve"`
	} `json:"vulnerabilities"`
}

// scan completes every page of a fixed modification window. The caller advances
// its cursor only after success; partial-page events are deduplicated on retry.
func (m *monitor) scan(ctx context.Context, start, end time.Time, visit func(CVE) error) (int, error) {
	index, total := 0, -1
	for {
		query := url.Values{
			"lastModStartDate": {start.Format("2006-01-02T15:04:05.000Z")},
			"lastModEndDate":   {end.Format("2006-01-02T15:04:05.000Z")},
			"resultsPerPage":   {"100"},
			"startIndex":       {strconv.Itoa(index)},
		}
		result, err := m.fetch(ctx, query)
		if err != nil {
			return index, err
		}
		if total == -1 {
			total = result.TotalResults
		}
		if result.StartIndex != index || result.TotalResults != total || len(result.Vulnerabilities) != result.ResultsPerPage || index+len(result.Vulnerabilities) > total {
			return index, fmt.Errorf("NVD pagination changed or is inconsistent; retrying window on next poll")
		}
		for _, item := range result.Vulnerabilities {
			cve := item.CVE
			if !cveIDPattern.MatchString(cve.ID) {
				return index, fmt.Errorf("invalid NVD CVE identifier")
			}
			modified, err := parseTimestamp(cve.LastModified)
			if err != nil {
				return index, err
			}
			if modified.Before(start) || modified.After(end) {
				return index, fmt.Errorf("NVD record outside requested modification window")
			}
			if err := visit(cve); err != nil {
				return index, err
			}
		}
		index += len(result.Vulnerabilities)
		if index == total {
			return index, nil
		}
		if len(result.Vulnerabilities) == 0 {
			return index, fmt.Errorf("NVD pagination stopped before all records were read")
		}
	}
}

func (m *monitor) fetch(ctx context.Context, query url.Values) (page, error) {
	// Six seconds between requests, including retries, respects the keyless rate.
	for attempt := 0; attempt < 3; attempt++ {
		if delay := time.Until(m.nextRequest); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()

				return page{}, ctx.Err()
			case <-timer.C:
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nvdEndpoint+"?"+query.Encode(), nil)
		if err != nil {
			return page{}, err
		}
		req.Header.Set("User-Agent", "monitord-cve")
		req.Header.Set("Accept", "application/json")
		if m.apiKey != "" {
			req.Header.Set("apiKey", m.apiKey)
		}
		m.nextRequest = time.Now().Add(6 * time.Second)
		response, err := m.client.Do(req)
		if err != nil {
			return page{}, fmt.Errorf("NVD request: %w", err)
		}
		if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			_ = response.Body.Close()
			delay := time.Duration(1<<attempt) * 6 * time.Second
			if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
				delay = max(delay, time.Duration(seconds)*time.Second)
			} else if when, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
				delay = max(delay, time.Until(when))
			}
			m.nextRequest = time.Now().Add(delay)
			if delay > time.Minute || attempt == 2 {
				return page{}, fmt.Errorf("NVD HTTP %d; window retained for retry", response.StatusCode)
			}

			continue
		}
		return decodePage(response)
	}

	return page{}, fmt.Errorf("NVD retry limit reached")
}

func decodePage(response *http.Response) (page, error) {
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return page{}, fmt.Errorf("NVD HTTP %d", response.StatusCode)
	}
	const maxBody = 32 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return page{}, fmt.Errorf("read NVD response: %w", err)
	}
	if len(raw) > maxBody {
		return page{}, fmt.Errorf("NVD response exceeds 32 MiB")
	}
	var result page
	if err := json.Unmarshal(raw, &result); err != nil {
		return page{}, fmt.Errorf("decode NVD response: %w", err)
	}
	if result.Format != "NVD_CVE" || result.Version != "2.0" || result.TotalResults < 0 || result.Vulnerabilities == nil {
		return page{}, fmt.Errorf("invalid NVD response envelope")
	}

	return result, nil
}

func parseTimestamp(value string) (time.Time, error) {
	// NVD normally supplies UTC timestamps without an explicit offset.
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid NVD timestamp %q", value)
}

// Detail is a normalized, material projection: ordering and indexing metadata
// do not generate alerts. A content revision can still include new references.
type Detail struct {
	Description string
	Products    []Product
	Affected    []string
	References  []string
	Metrics     []string
	Score       *float64
	ScoreLabel  string
	KEV         bool
	Rejected    bool
}

func summarize(cve CVE) Detail {
	detail := Detail{
		KEV:      cve.KEVAdded != "",
		Rejected: strings.EqualFold(cve.Status, "Rejected"),
	}
	for _, description := range cve.Descriptions {
		if description.Lang == "en" {
			detail.Description = strings.TrimSpace(description.Value)

			break
		}
	}
	for _, metrics := range [][]Metric{cve.Metrics.V40, cve.Metrics.V31, cve.Metrics.V30, cve.Metrics.V2} {
		for _, metric := range metrics {
			if metric.Data.Score == nil || *metric.Data.Score < 0 || *metric.Data.Score > 10 {
				continue
			}
			label := fmt.Sprintf("CVSS %s · %s", metric.Data.Version, metric.Source)
			detail.Metrics = append(detail.Metrics, fmt.Sprintf("%.1f %s %s", *metric.Data.Score, label, metric.Data.Vector))
			if detail.Score == nil || *metric.Data.Score > *detail.Score || (*metric.Data.Score == *detail.Score && label < detail.ScoreLabel) {
				detail.Score = metric.Data.Score
				detail.ScoreLabel = label
			}
		}
	}
	for _, configuration := range cve.Configurations {
		for _, node := range configuration.Nodes {
			for _, match := range node.Matches {
				if !match.Vulnerable {
					continue
				}
				parts := splitCPE(match.Criteria)
				if len(parts) != 13 || parts[0] != "cpe" || parts[1] != "2.3" {
					continue
				}
				detail.Products = append(detail.Products, Product{Vendor: parts[3], Product: parts[4]})
				affected := parts[3] + "/" + parts[4]
				if parts[5] != "*" && parts[5] != "-" {
					affected += " " + parts[5]
				}
				for _, bound := range []struct{ operator, version string }{
					{">=", match.StartIncluding}, {">", match.StartExcluding},
					{"<=", match.EndIncluding}, {"<", match.EndExcluding},
				} {
					if bound.version != "" {
						affected += " " + bound.operator + " " + bound.version
					}
				}
				detail.Affected = append(detail.Affected, affected)
			}
		}
	}
	for _, source := range cve.Affected {
		for _, product := range source.Data {
			if product.Vendor != "" && product.Product != "" && product.Vendor != "n/a" && product.Product != "n/a" {
				detail.Products = append(detail.Products, Product{Vendor: product.Vendor, Product: product.Product})
			}
		}
	}
	for _, reference := range cve.References {
		if reference.URL != "" {
			// Only the literal URL is inspected; reference targets are never fetched.
			detail.References = append(detail.References, reference.URL)
		}
	}
	for index := range detail.Products {
		detail.Products[index].Vendor = strings.ToLower(detail.Products[index].Vendor)
		detail.Products[index].Product = strings.ToLower(detail.Products[index].Product)
	}
	slices.SortFunc(detail.Products, func(a, b Product) int {
		if order := strings.Compare(a.Vendor, b.Vendor); order != 0 {
			return order
		}

		return strings.Compare(a.Product, b.Product)
	})
	detail.Products = slices.Compact(detail.Products)
	for _, values := range []*[]string{&detail.Affected, &detail.References, &detail.Metrics} {
		slices.Sort(*values)
		*values = slices.Compact(*values)
	}

	return detail
}

func splitCPE(value string) []string {
	var parts []string
	var part strings.Builder
	escaped := false
	for _, char := range value {
		if escaped {
			part.WriteRune(char)
			escaped = false
		} else if char == '\\' {
			escaped = true
		} else if char == ':' {
			parts = append(parts, part.String())
			part.Reset()
		} else {
			part.WriteRune(char)
		}
	}
	if escaped {
		return nil
	}

	return append(parts, part.String())
}

func matchingRules(rules []Rule, detail Detail) []string {
	var names []string
	text := strings.ToLower(detail.Description)
	for _, rule := range rules {
		if containsKeyword(text, rule.ExcludeKeywords) {
			continue
		}
		matched := containsKeyword(text, rule.Keywords)
		for _, want := range rule.Products {
			for _, product := range detail.Products {
				matched = matched || (strings.EqualFold(product.Vendor, want.Vendor) && strings.EqualFold(product.Product, want.Product))
			}
		}
		for _, prefix := range rule.ReferencePrefixes {
			for _, reference := range detail.References {
				matched = matched || referenceMatches(reference, prefix)
			}
		}
		if matched {
			names = append(names, rule.Name)
		}
	}
	slices.Sort(names)

	return names
}

func containsKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}

	return false
}

func referenceMatches(reference, prefix string) bool {
	ref, err := url.Parse(reference)
	if err != nil || ref.User != nil || (ref.Scheme != "https" && ref.Scheme != "http") {
		return false
	}
	want, err := url.Parse(prefix)
	if err != nil || !strings.EqualFold(ref.Host, want.Host) {
		return false
	}
	path := strings.TrimRight(want.Path, "/")

	return ref.Path == path || strings.HasPrefix(ref.Path, path+"/")
}

func cveEvent(cve CVE, detail Detail, labels []string, known, withdrawn bool) monitord.Event {
	kind := "New match"
	if known {
		kind = "Updated"
	}
	severity, color := monitord.SeverityInfo, 0x6C9BCF
	score := "Not yet scored"
	if detail.Score != nil {
		score = fmt.Sprintf("%.1f / 10\n%s", *detail.Score, detail.ScoreLabel)
		if *detail.Score >= 7 {
			severity, color = monitord.SeverityWarn, 0xD9A441
		}
		if *detail.Score >= 9 {
			severity, color = monitord.SeverityCritical, 0xD9534F
		}
	}
	if detail.KEV {
		severity, color = monitord.SeverityCritical, 0xD9534F
	}
	if withdrawn {
		kind = "Withdrawn"
		severity, color = monitord.SeverityInfo, 0x7C8999
	}
	fields := []monitord.EventField{
		{Name: "Highest reported CVSS", Value: clip(score, 300), Inline: true},
	}
	if len(labels) > 0 {
		fields = append(fields, monitord.EventField{Name: "Matched rules", Value: clip(strings.Join(labels, ", "), 500), Inline: true})
	}
	if detail.KEV {
		fields = append(fields, monitord.EventField{Name: "Known exploited", Value: "Listed in CISA KEV (via NVD)", Inline: true})
	}
	if len(detail.Affected) > 0 {
		fields = append(fields, monitord.EventField{Name: "Affected ranges (NVD)", Value: clip(strings.Join(detail.Affected, "\n"), 900)})
	}
	fields = append(fields, monitord.EventField{Name: "Source updated (UTC)", Value: cve.LastModified})
	link := "https://nvd.nist.gov/vuln/detail/" + cve.ID

	return monitord.Event{
		Title:       kind + " · " + cve.ID,
		Severity:    severity,
		URL:         link,
		Description: clip(detail.Description, 1200),
		Color:       color,
		Fields:      fields,
		Footer:      "NVD · Product match, not confirmation that an installed version is vulnerable",
		Mentions:    []string{},
	}
}

func clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}

	return value
}
