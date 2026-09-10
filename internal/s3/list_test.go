package s3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// newListTestServer 启动一个协议级 ListObjectsV2 模拟服务器。
//
// 它维护 total 个键为 fmt.Sprintf(keyFormat, i) 的虚拟对象（i 从 0
// 开始），完整实现 max-keys / prefix / continuation-token /
// IsTruncated / NextContinuationToken 语义。continuation token 是
// "offset-<n>"：n 为已跳过的匹配键数量（绝对偏移），与 page-size 无关。
// nextToken 为 nil 时走标准 token 生成；传入自定义函数可模拟缺失、
// 重复等 provider 缺陷。
func newListTestServer(t *testing.T, total int, keyFormat string, nextToken func(offset int, lastToken string) string) (*listRequestRecorder, *Client) {
	t.Helper()

	rec := &listRequestRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r.URL.Query())

		query := r.URL.Query()
		if query.Get("list-type") != "2" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		maxKeys, _ := strconv.Atoi(query.Get("max-keys"))
		if maxKeys <= 0 {
			maxKeys = 1000
		}
		prefix := query.Get("prefix")

		matched := make([]string, 0, total)
		for i := 0; i < total; i++ {
			key := fmt.Sprintf(keyFormat, i)
			if strings.HasPrefix(key, prefix) {
				matched = append(matched, key)
			}
		}

		start := 0
		if token := query.Get("continuation-token"); token != "" {
			var offset int
			if _, err := fmt.Sscanf(token, "offset-%d", &offset); err != nil || offset < 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			start = offset
		}
		if start > len(matched) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		end := start + maxKeys
		if end > len(matched) {
			end = len(matched)
		}
		pageKeys := matched[start:end]
		truncated := end < len(matched)

		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
		b.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
		b.WriteString(`<Name>test-bucket</Name>`)
		b.WriteString(`<Prefix>` + prefix + `</Prefix>`)
		b.WriteString(`<MaxKeys>` + strconv.Itoa(maxKeys) + `</MaxKeys>`)
		b.WriteString(`<KeyCount>` + strconv.Itoa(len(pageKeys)) + `</KeyCount>`)
		b.WriteString(`<IsTruncated>` + strconv.FormatBool(truncated) + `</IsTruncated>`)
		if truncated {
			last := "offset-" + strconv.Itoa(end)
			next := last
			if nextToken != nil {
				next = nextToken(end, last)
			}
			b.WriteString(`<NextContinuationToken>` + next + `</NextContinuationToken>`)
		}
		for _, key := range pageKeys {
			b.WriteString(`<Contents>`)
			b.WriteString(`<Key>` + key + `</Key>`)
			b.WriteString(`<LastModified>2024-01-01T00:00:00.000Z</LastModified>`)
			b.WriteString(`<ETag>&#34;etag&#34;</ETag>`)
			b.WriteString(`<Size>` + strconv.Itoa(len(key)) + `</Size>`)
			b.WriteString(`<StorageClass>STANDARD</StorageClass>`)
			b.WriteString(`</Contents>`)
		}
		b.WriteString(`</ListBucketResult>`)

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)

	cfg := &Config{
		Endpoint:        srv.URL,
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		Region:          "us-east-1",
		Bucket:          "test-bucket",
		ForcePathStyle:  true,
	}
	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return rec, client
}

// listRequestRecorder 记录每个 ListObjectsV2 请求的关键查询参数。
type listRequestRecorder struct {
	requests []url.Values
}

func (r *listRequestRecorder) record(q url.Values) {
	r.requests = append(r.requests, q)
}

// maxKeysValues 返回每次请求的 max-keys 序列。
func (r *listRequestRecorder) maxKeysValues() []string {
	values := make([]string, 0, len(r.requests))
	for _, q := range r.requests {
		values = append(values, q.Get("max-keys"))
	}
	return values
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestListPaginationCounts 覆盖 0/1/999/1000/1001/2001 对象在单页与
// --all 模式下的分页边界。
func TestListPaginationCounts(t *testing.T) {
	tests := []struct {
		name         string
		total        int
		pageSize     int32
		all          bool
		wantCount    int
		wantPages    int
		wantComplete bool
		wantToken    bool
	}{
		{"0 objects single page", 0, 0, false, 0, 1, true, false},
		{"1 object single page", 1, 0, false, 1, 1, true, false},
		{"999 objects single page fits MaxKeys 1000", 999, 0, false, 999, 1, true, false},
		{"page-size 999 truncates 1001 objects", 1001, 999, false, 999, 1, false, true},
		{"1000 objects single page", 1000, 0, false, 1000, 1, true, false},
		{"1001 objects single page", 1001, 0, false, 1000, 1, false, true},
		{"2001 objects single page", 2001, 0, false, 1000, 1, false, true},
		{"999 objects all", 999, 0, true, 999, 1, true, false},
		{"1000 objects all", 1000, 0, true, 1000, 1, true, false},
		{"1001 objects all", 1001, 0, true, 1001, 2, true, false},
		{"2001 objects all", 2001, 0, true, 2001, 3, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := newListTestServer(t, tt.total, "obj-%06d", nil)
			result, err := List(context.Background(), client, &ListOptions{PageSize: tt.pageSize, All: tt.all})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if result.Count != tt.wantCount || len(result.Objects) != tt.wantCount {
				t.Fatalf("count = %d/%d, want %d", result.Count, len(result.Objects), tt.wantCount)
			}
			if result.Pages != tt.wantPages {
				t.Errorf("pages = %d, want %d", result.Pages, tt.wantPages)
			}
			if result.Complete != tt.wantComplete {
				t.Errorf("complete = %v, want %v", result.Complete, tt.wantComplete)
			}
			hasToken := result.NextContinuationToken != ""
			if hasToken != tt.wantToken {
				t.Errorf("next token present = %v, want %v", hasToken, tt.wantToken)
			}
			if result.SchemaVersion != ListSchemaVersion {
				t.Errorf("schema_version = %q, want %q", result.SchemaVersion, ListSchemaVersion)
			}
			if result.Bucket != "test-bucket" {
				t.Errorf("bucket = %q, want test-bucket", result.Bucket)
			}
		})
	}
}

// TestListPageSizeControlsMaxKeys --page-size 控制单次请求的 MaxKeys。
func TestListPageSizeControlsMaxKeys(t *testing.T) {
	t.Run("page-size 500 paginates 2001 objects in 5 pages", func(t *testing.T) {
		rec, client := newListTestServer(t, 2001, "obj-%06d", nil)
		result, err := List(context.Background(), client, &ListOptions{PageSize: 500, All: true})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if result.Count != 2001 || result.Pages != 5 || !result.Complete {
			t.Fatalf("result = count %d, pages %d, complete %v", result.Count, result.Pages, result.Complete)
		}
		// 无 limit 时每次请求都使用完整 page-size，最后一页由服务端
		// 自然截断（只返回剩余的 1 个对象）
		if got, want := rec.maxKeysValues(), []string{"500", "500", "500", "500", "500"}; !equalStrings(got, want) {
			t.Errorf("max-keys per request = %v, want %v", got, want)
		}
	})

	t.Run("page-size 0 归一化为协议默认 1000", func(t *testing.T) {
		_, client := newListTestServer(t, 0, "obj-%06d", nil)
		result, err := List(context.Background(), client, &ListOptions{PageSize: 0})
		if err != nil {
			t.Fatalf("List(page-size 0): %v", err)
		}
		if result.Pages != 1 {
			t.Fatalf("pages = %d, want 1", result.Pages)
		}
	})

	// domain 是参数规则的唯一事实来源：超范围 page-size 必须报
	// ErrInvalidOptions，绝不静默收敛为 1000（否则 domain 直调与
	// CLI adapter 走出不同契约）
	t.Run("invalid page-size is rejected", func(t *testing.T) {
		_, client := newListTestServer(t, 0, "obj-%06d", nil)
		for _, size := range []int32{-1, 1001, 5000} {
			_, err := List(context.Background(), client, &ListOptions{PageSize: size})
			if err == nil || !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("List(page-size %d) err = %v, want ErrInvalidOptions", size, err)
			}
		}
	})

	t.Run("negative limit is rejected", func(t *testing.T) {
		_, client := newListTestServer(t, 0, "obj-%06d", nil)
		_, err := List(context.Background(), client, &ListOptions{Limit: -5})
		if err == nil || !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("err = %v, want ErrInvalidOptions", err)
		}
	})
}

// TestListSpecialKeys 键含 Unicode / 空格 / + / % 时完整往返。
func TestListSpecialKeys(t *testing.T) {
	keys := []string{
		"图片/照片 001.png",
		"a+b/sum.png",
		"100%/done.png",
		"emoji-🚀.png",
		"plain.png",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
		b.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
		b.WriteString(`<Name>test-bucket</Name><Prefix></Prefix>`)
		b.WriteString(`<KeyCount>` + strconv.Itoa(len(keys)) + `</KeyCount>`)
		b.WriteString(`<IsTruncated>false</IsTruncated>`)
		for _, key := range keys {
			b.WriteString(`<Contents><Key>` + key + `</Key><LastModified>2024-01-01T00:00:00.000Z</LastModified><ETag>&#34;e&#34;</ETag><Size>1</Size><StorageClass>STANDARD</StorageClass></Contents>`)
		}
		b.WriteString(`</ListBucketResult>`)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)

	cfg := &Config{
		Endpoint: srv.URL, AccessKeyID: "ak", SecretAccessKey: "sk",
		Region: "us-east-1", Bucket: "test-bucket", ForcePathStyle: true,
	}
	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result, err := List(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Count != len(keys) {
		t.Fatalf("count = %d, want %d", result.Count, len(keys))
	}
	gotKeys := make(map[string]bool, len(result.Objects))
	for _, obj := range result.Objects {
		gotKeys[obj.Key] = true
	}
	for _, key := range keys {
		if !gotKeys[key] {
			t.Errorf("key %q missing from result", key)
		}
	}
}

// TestListLimitTruncation --limit 截断：complete=false、token 指向最后
// 输出对象的下一个键，从该 token 恢复后与一次性 --all 结果一致。
func TestListLimitTruncation(t *testing.T) {
	rec, client := newListTestServer(t, 2001, "obj-%06d", nil)

	result, err := List(context.Background(), client, &ListOptions{Limit: 1500, All: true})
	if err != nil {
		t.Fatalf("List(limit): %v", err)
	}
	if result.Count != 1500 {
		t.Fatalf("count = %d, want 1500", result.Count)
	}
	if result.Complete {
		t.Fatal("limit-truncated result must not be complete")
	}
	if result.NextContinuationToken == "" {
		t.Fatal("limit-truncated result must carry next_continuation_token")
	}
	// 1500 = 1000 + 500：第二次请求 MaxKeys 收缩到剩余配额，
	// 保证 token 恰好指向最后输出对象的下一个键
	if got, want := rec.maxKeysValues(), []string{"1000", "500"}; !equalStrings(got, want) {
		t.Errorf("max-keys sequence = %v, want %v", got, want)
	}
	if last := result.Objects[len(result.Objects)-1].Key; last != "obj-001499" {
		t.Errorf("last key = %q, want obj-001499", last)
	}

	// 从 token 恢复，补齐剩余对象且不跳过任何键
	resumed, err := List(context.Background(), client, &ListOptions{
		ContinuationToken: result.NextContinuationToken,
		All:               true,
	})
	if err != nil {
		t.Fatalf("List(resume): %v", err)
	}
	if !resumed.Complete {
		t.Fatal("resumed listing must complete")
	}
	if resumed.Count != 2001-1500 {
		t.Fatalf("resumed count = %d, want %d", resumed.Count, 2001-1500)
	}
	if first := resumed.Objects[0].Key; first != "obj-001500" {
		t.Errorf("resumed first key = %q, want obj-001500（token 不得跳过对象）", first)
	}

	// 合并两段与一次性 --all 完全一致
	whole, err := List(context.Background(), client, &ListOptions{All: true})
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	merged := append(append([]ObjectInfo{}, result.Objects...), resumed.Objects...)
	if len(merged) != len(whole.Objects) {
		t.Fatalf("merged %d objects, want %d", len(merged), len(whole.Objects))
	}
	for i := range merged {
		if merged[i].Key != whole.Objects[i].Key {
			t.Fatalf("merged[%d].Key = %q, want %q", i, merged[i].Key, whole.Objects[i].Key)
		}
	}
}

// TestListLimitExactBoundary limit 恰好等于对象总数：遍历自然结束，
// complete=true 且无 token。
func TestListLimitExactBoundary(t *testing.T) {
	_, client := newListTestServer(t, 1000, "obj-%06d", nil)
	result, err := List(context.Background(), client, &ListOptions{Limit: 1000, All: true})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Count != 1000 || !result.Complete || result.NextContinuationToken != "" {
		t.Fatalf("result = count %d, complete %v, token %q", result.Count, result.Complete, result.NextContinuationToken)
	}
}

// TestListLimitSmallerThanOnePage limit 小于一页容量：单次请求收缩
// MaxKeys，一次请求即完成截断。
func TestListLimitSmallerThanOnePage(t *testing.T) {
	rec, client := newListTestServer(t, 2001, "obj-%06d", nil)
	result, err := List(context.Background(), client, &ListOptions{Limit: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Count != 5 || result.Complete || result.NextContinuationToken == "" {
		t.Fatalf("result = count %d, complete %v, token %q", result.Count, result.Complete, result.NextContinuationToken)
	}
	if got, want := rec.maxKeysValues(), []string{"5"}; !equalStrings(got, want) {
		t.Errorf("max-keys sequence = %v, want %v", got, want)
	}
}

// TestListFromToken 从指定 token 恢复：请求携带 ContinuationToken 且
// complete 语义以该 token 为起点。
func TestListFromToken(t *testing.T) {
	rec, client := newListTestServer(t, 3000, "obj-%06d", nil)

	result, err := List(context.Background(), client, &ListOptions{
		ContinuationToken: "offset-1000",
		PageSize:          1000,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Count != 1000 {
		t.Fatalf("count = %d, want 1000", result.Count)
	}
	if result.Complete {
		t.Error("listing starting mid-bucket must report complete=false when more pages follow")
	}
	if first := result.Objects[0].Key; first != "obj-001000" {
		t.Errorf("first key = %q, want obj-001000", first)
	}
	if got := rec.requests[0].Get("continuation-token"); got != "offset-1000" {
		t.Errorf("request continuation-token = %q, want offset-1000", got)
	}
}

// TestListIncompleteTokenDefects IsTruncated=true 但 token 缺失、重复
// 时返回 ErrIncompleteList，绝不输出半份成功结果。
func TestListIncompleteTokenDefects(t *testing.T) {
	t.Run("missing token", func(t *testing.T) {
		_, client := newListTestServer(t, 2001, "obj-%06d", func(int, string) string { return "" })
		result, err := List(context.Background(), client, &ListOptions{All: true})
		if err == nil || !strings.Contains(err.Error(), ErrIncompleteList.Error()) {
			t.Fatalf("err = %v, want %v", err, ErrIncompleteList)
		}
		if result != nil {
			t.Errorf("partial result must not be returned, got %+v", result)
		}
	})

	t.Run("repeated token does not loop forever", func(t *testing.T) {
		_, client := newListTestServer(t, 2001, "obj-%06d", func(int, string) string { return "offset-0" })
		_, err := List(context.Background(), client, &ListOptions{All: true})
		if err == nil || !strings.Contains(err.Error(), ErrIncompleteList.Error()) {
			t.Fatalf("err = %v, want %v", err, ErrIncompleteList)
		}
	})

	t.Run("limit truncation without token fails", func(t *testing.T) {
		_, client := newListTestServer(t, 2001, "obj-%06d", func(int, string) string { return "" })
		_, err := List(context.Background(), client, &ListOptions{Limit: 5, All: true})
		if err == nil || !strings.Contains(err.Error(), ErrIncompleteList.Error()) {
			t.Fatalf("err = %v, want %v", err, ErrIncompleteList)
		}
	})
}

// TestListMiddlePageFails 中间页失败：整个命令失败，不返回已收集对象。
func TestListMiddlePageFails(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>test-bucket</Name><Prefix></Prefix><MaxKeys>1000</MaxKeys><KeyCount>1</KeyCount><IsTruncated>true</IsTruncated><NextContinuationToken>offset-1</NextContinuationToken><Contents><Key>a.png</Key><LastModified>2024-01-01T00:00:00.000Z</LastModified><ETag>&#34;e&#34;</ETag><Size>1</Size><StorageClass>STANDARD</StorageClass></Contents></ListBucketResult>`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<Error><Code>InternalError</Code></Error>`))
	}))
	t.Cleanup(srv.Close)

	cfg := &Config{
		Endpoint: srv.URL, AccessKeyID: "ak", SecretAccessKey: "sk",
		Region: "us-east-1", Bucket: "test-bucket", ForcePathStyle: true,
	}
	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result, err := List(context.Background(), client, &ListOptions{All: true})
	if err == nil {
		t.Fatal("expected middle-page failure")
	}
	if result != nil {
		t.Errorf("partial result must not be returned, got %d objects", result.Count)
	}
}

// TestListEmptyObjectsJSONArray 空结果输出 "objects": []（非 null）。
func TestListEmptyObjectsJSONArray(t *testing.T) {
	_, client := newListTestServer(t, 0, "obj-%06d", nil)
	result, err := List(context.Background(), client, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(formatJSON(result), `"objects": []`) {
		t.Errorf("empty list JSON must carry objects: [], got %s", formatJSON(result))
	}

	var decoded ListResult
	if err := json.Unmarshal([]byte(FormatOutput(result, "json")), &decoded); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if decoded.SchemaVersion != ListSchemaVersion || decoded.Count != 0 || len(decoded.Objects) != 0 {
		t.Fatalf("decoded = %+v", decoded)
	}
}
