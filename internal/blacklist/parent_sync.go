package blacklist

import (
	"encoding/json"
	"fmt"
	"io"
	"lemwood_mirror/internal/db"
	"net/http"
	"time"
)

// SyncFromParent 从父节点拉取黑名单全量并本地对账（source=parent）。
// 父节点地址形如 https://main.example（不含路径），key 为节点互访密钥。
func SyncFromParent(parentURL, key string, insecureSkipVerify bool) error {
	if parentURL == "" || key == "" {
		return nil
	}
	syncMu.Lock()
	defer syncMu.Unlock()

	var transport http.RoundTripper = http.DefaultTransport
	if insecureSkipVerify {
		transport = insecureTransport()
	}
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequest(http.MethodGet, parentURL+"/api/v2/node/blacklist", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Node-Key", key)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("父节点返回状态码 %d", resp.StatusCode)
	}

	var envelope struct {
		Data []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return err
	}

	if err := db.ReplaceParentBlacklist(envelope.Data); err != nil {
		return err
	}
	return nil
}
