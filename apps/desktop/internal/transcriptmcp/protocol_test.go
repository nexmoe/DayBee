package transcriptmcp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nexmoe/DayBee/apps/desktop/internal/protocol"
)

func protocolRequests(t *testing.T) func(string, map[string]any) *jsonrpc.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	a, b := mcp.NewInMemoryTransports()
	server, err := NewServer("test", func(context.Context) ([]protocol.RecordingHistoryItem, error) {
		return nil, nil
	}).Connect(ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := b.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close(); _ = server.Close() })
	id := 0
	return func(method string, params map[string]any) *jsonrpc.Response {
		t.Helper()
		id++
		data, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
		if err != nil {
			t.Fatal(err)
		}
		request, err := jsonrpc.DecodeMessage(data)
		if err != nil {
			t.Fatal(err)
		}
		if err := connection.Write(ctx, request); err != nil {
			t.Fatal(err)
		}
		message, err := connection.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		response, ok := message.(*jsonrpc.Response)
		if !ok || response.ID != request.(*jsonrpc.Request).ID {
			t.Fatalf("unexpected response: %+v", message)
		}
		return response
	}
}

func requestMeta() map[string]any {
	return map[string]any{
		mcp.MetaKeyProtocolVersion:    "2026-07-28",
		mcp.MetaKeyClientCapabilities: map[string]any{},
		mcp.MetaKeyClientInfo:         map[string]any{"name": "protocol-test", "version": "1"},
	}
}

func TestMCP20260728ServesRequestsWithoutInitialization(t *testing.T) {
	request := protocolRequests(t)
	// Discovery is optional: a modern client can list tools as its first request.
	for _, method := range []string{"tools/list", "server/discover", "resources/list", "resources/templates/list", "resources/read"} {
		t.Run(method, func(t *testing.T) {
			params := map[string]any{"_meta": requestMeta()}
			if method == "resources/read" {
				params["uri"] = indexURI
			}
			response := request(method, params)
			if response.Error != nil {
				t.Fatal(response.Error)
			}
			var result struct {
				ResultType        string          `json:"resultType"`
				TTLMs             *int            `json:"ttlMs"`
				CacheScope        string          `json:"cacheScope"`
				Meta              mcp.Meta        `json:"_meta"`
				SupportedVersions []string        `json:"supportedVersions"`
				Capabilities      json.RawMessage `json:"capabilities"`
			}
			if err := json.Unmarshal(response.Result, &result); err != nil {
				t.Fatal(err)
			}
			if result.ResultType != "complete" || result.TTLMs == nil || *result.TTLMs != 0 || result.CacheScope != "private" {
				t.Fatalf("unexpected modern result or cache policy: %s", response.Result)
			}
			info, ok := result.Meta[mcp.MetaKeyServerInfo].(map[string]any)
			if !ok || info["name"] != "daybee" || info["version"] != "test" {
				t.Fatalf("server identity missing: %s", response.Result)
			}
			if method == "server/discover" && (!slices.Equal(result.SupportedVersions, []string{"2026-07-28"}) || len(result.Capabilities) == 0) {
				t.Fatalf("unexpected discovery: %s", response.Result)
			}
		})
	}
}

func TestMCP20260728ValidatesMetadataOnEveryRequest(t *testing.T) {
	request := protocolRequests(t)
	for _, missing := range []string{mcp.MetaKeyProtocolVersion, mcp.MetaKeyClientCapabilities} {
		t.Run(missing, func(t *testing.T) {
			// A previous valid request must not supply metadata for a later request.
			if response := request("server/discover", map[string]any{"_meta": requestMeta()}); response.Error != nil {
				t.Fatal(response.Error)
			}
			meta := requestMeta()
			delete(meta, missing)
			response := request("tools/list", map[string]any{"_meta": meta})
			var rpcError *jsonrpc.Error
			if !errors.As(response.Error, &rpcError) || rpcError.Code != jsonrpc.CodeInvalidParams {
				t.Fatalf("missing metadata accepted: %+v", response)
			}
		})
	}
	for _, params := range []map[string]any{nil, {}, {"_meta": nil}, {"_meta": map[string]any{mcp.MetaKeyProtocolVersion: 1}}} {
		response := request("tools/list", params)
		var rpcError *jsonrpc.Error
		if !errors.As(response.Error, &rpcError) || rpcError.Code != jsonrpc.CodeInvalidParams {
			t.Fatalf("invalid request metadata accepted: %+v", response)
		}
	}
	meta := requestMeta()
	meta[mcp.MetaKeyProtocolVersion] = "2099-01-01"
	response := request("server/discover", map[string]any{"_meta": meta})
	var rpcError *jsonrpc.Error
	if !errors.As(response.Error, &rpcError) || rpcError.Code != mcp.CodeUnsupportedProtocolVersion {
		t.Fatalf("unsupported protocol version accepted: %+v", response)
	}
	var data mcp.UnsupportedProtocolVersionData
	if err := json.Unmarshal(rpcError.Data, &data); err != nil || data.Requested != "2099-01-01" || !slices.Equal(data.Supported, []string{"2026-07-28"}) {
		t.Fatalf("version renegotiation data=%+v err=%v", data, err)
	}
	response = request("resources/read", map[string]any{"_meta": requestMeta(), "uri": indexURI + "/unknown"})
	if !errors.As(response.Error, &rpcError) || rpcError.Code != jsonrpc.CodeInvalidParams {
		t.Fatalf("unexpected missing-resource error: %+v", response)
	}
}

func TestMCPRejectsLegacyInitialization(t *testing.T) {
	for _, version := range []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"} {
		t.Run(version, func(t *testing.T) {
			request := protocolRequests(t)
			response := request("initialize", map[string]any{
				"protocolVersion": version, "capabilities": map[string]any{},
				"clientInfo": map[string]any{"name": "legacy-test", "version": "1"},
			})
			var rpcError *jsonrpc.Error
			if !errors.As(response.Error, &rpcError) || rpcError.Code != mcp.CodeUnsupportedProtocolVersion || !strings.Contains(rpcError.Message, "2026-07-28") {
				t.Fatalf("legacy initialization was not explicitly rejected: %+v", response)
			}
			if response := request("tools/call", map[string]any{"name": "list_transcripts", "arguments": map[string]any{}}); response.Error == nil {
				t.Fatal("rejected legacy client could still read transcripts")
			}
			if response := request("tools/list", map[string]any{"_meta": requestMeta()}); response.Error != nil {
				t.Fatal("modern requests failed after rejected initialization", response.Error)
			}
			meta := requestMeta()
			meta[mcp.MetaKeyProtocolVersion] = version
			response = request("tools/list", map[string]any{"_meta": meta})
			if !errors.As(response.Error, &rpcError) || rpcError.Code != mcp.CodeUnsupportedProtocolVersion {
				t.Fatalf("legacy per-request version accepted: %+v", response)
			}
			var data mcp.UnsupportedProtocolVersionData
			if err := json.Unmarshal(rpcError.Data, &data); err != nil || data.Requested != version || !slices.Equal(data.Supported, []string{"2026-07-28"}) {
				t.Fatalf("unexpected supported versions: %+v err=%v", data, err)
			}
		})
	}
}
