// backend/verify.go - checks Telebirr deposits with the Verify.ET API (https://verify.et/docs/api)
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// waitMs keeps the whole check under the 15 s the bot gives one message.
const verifyURL = "https://verify.et/api/verify?waitMs=7000"

// flexNum reads an amount that Verify.ET may send as a number (1500) or a string ("1,500.00").
type flexNum float64

func (f *flexNum) UnmarshalJSON(b []byte) error {
	s := strings.ReplaceAll(strings.Trim(strings.TrimSpace(string(b)), `"`), ",", "")
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		*f = flexNum(v)
	}
	return nil
}

type verifyItem struct {
	Bank            string  `json:"bank"`
	Status          string  `json:"status"`
	Verified        bool    `json:"verified"`
	Amount          flexNum `json:"amount"`
	Currency        string  `json:"currency"`
	SenderName      string  `json:"senderName"`
	ReceiverName    string  `json:"receiverName"`
	ReceiverAccount string  `json:"receiverAccount"`
	Settlement      *struct {
		Matched bool `json:"matched"`
	} `json:"settlementAccountMatch"`
}

type recvState int

const (
	recvUnknown recvState = iota // receiver could not be read: let the admin decide
	recvOK                       // paid into our Telebirr account
	recvWrong                    // paid to somebody else
)

func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// receiverCheck compares the receipt's receiver with TELEBIRR_NUMBER.
func (it *verifyItem) receiverCheck() recvState {
	if it.Settlement != nil && it.Settlement.Matched {
		return recvOK
	}
	got, want := onlyDigits(it.ReceiverAccount), onlyDigits(cfg.TelebirrNumber)
	if len(got) < 4 || len(want) < 4 {
		return recvUnknown
	}
	if got[len(got)-4:] == want[len(want)-4:] { // receipts mask the middle: 2519****0474
		return recvOK
	}
	return recvWrong
}

type verifyOutcome int

const (
	verifyDown    verifyOutcome = iota // Verify.ET unreachable / out of credits / still queued -> manual review
	verifyOK                           // receipt is real
	verifyInvalid                      // receipt not found or not valid
	verifyBusy                         // same user asked again too fast
)

type verifyResult struct {
	Outcome verifyOutcome
	Item    *verifyItem
	Message string
}

var (
	verifyMu   sync.Mutex
	verifyLast = map[int64]time.Time{}
)

func verifyTelebirr(ctx context.Context, uid int64, tx string) verifyResult {
	// one check per user every 10 s, so nobody can burn your Verify.ET credits
	verifyMu.Lock()
	if t, ok := verifyLast[uid]; ok && time.Since(t) < 10*time.Second {
		verifyMu.Unlock()
		return verifyResult{Outcome: verifyBusy}
	}
	verifyLast[uid] = time.Now()
	verifyMu.Unlock()

	body := map[string]string{"bank": "telebirr", "transactionNumber": tx}
	if cfg.TelebirrNumber != "" {
		body["settlementAccount"] = cfg.TelebirrNumber
	}
	b, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(ctx, 11*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, bytes.NewReader(b))
	if err != nil {
		return verifyResult{}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cfg.VerifyETKey)
	req.Header.Set("Idempotency-Key", "dep-"+tx+"-"+strconv.FormatInt(time.Now().Unix(), 10))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("verify.et:", err)
		return verifyResult{}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var out struct {
		Message string       `json:"message"`
		Data    []verifyItem `json:"data"`
	}
	notFound := verifyResult{Outcome: verifyInvalid, Message: "ትራንዛክሽኑ አልተገኘም ወይም አልተረጋገጠም። ቁጥሩን ያረጋግጡ ወይም ጥቂት ቆይተው እንደገና ይሞክሩ።"}
	switch resp.StatusCode {
	case http.StatusOK:
		if json.Unmarshal(raw, &out) != nil || len(out.Data) == 0 {
			log.Println("verify.et: unreadable 200:", string(raw[:min(len(raw), 300)]))
			return verifyResult{}
		}
		it := out.Data[0]
		if !it.Verified {
			return notFound
		}
		if it.Currency != "" && !strings.EqualFold(it.Currency, "ETB") {
			return notFound
		}
		return verifyResult{Outcome: verifyOK, Item: &it}
	case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
		return notFound
	default: // 202 still queued, 401/403 key problem, 402 no credits, 429, 5xx
		log.Println("verify.et: status", resp.StatusCode, string(raw[:min(len(raw), 300)]))
		return verifyResult{}
	}
}
