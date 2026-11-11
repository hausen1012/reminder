// about_service 提供关于页所需的版本信息与更新检查。
//
// 更新检查读取 GitHub 仓库的标签列表，取其中最大的版本号与当前构建版本比较。
// 自动检查每天运行一次，可在面板「设置 → 关于」关闭；关闭后只有手动检查会出网。
package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/reminder/backend/internal/logger"
	"github.com/reminder/backend/internal/version"
	"gorm.io/gorm"
)

// KeyUpdateCheckEnabled 是自动更新检查开关在 config 表中的 key；缺省视为开启。
const KeyUpdateCheckEnabled = "update_check_enabled"

const (
	// repoAPIURL 是 GitHub 仓库标签列表接口；测试通过 AboutService.APIURL 替换。
	repoAPIURL = "https://api.github.com/repos/hausen1012/reminder/tags"
	// repoTagURL 是单个标签的页面地址，用于「查看版本」链接。
	repoTagURL = "https://github.com/hausen1012/reminder/releases/tag/"
	// repoReleasesURL 是全部版本列表页面地址。
	repoReleasesURL = "https://github.com/hausen1012/reminder/releases"
	// updateCheckTimeout 是单次更新检查的请求超时。
	updateCheckTimeout = 15 * time.Second
	// tagListLimit 是单次拉取的标签数量，足够覆盖全部历史版本。
	tagListLimit = 100
)

// UpdateStatus 是对外的更新检查状态。
type UpdateStatus struct {
	// Enabled 表示是否自动检查更新。
	Enabled bool `json:"enabled"`
	// Comparable 表示当前构建版本能否参与版本比较；本地开发构建（dev）为 false。
	Comparable bool `json:"comparable"`
	// LatestVersion 是最近一次检查发现的最新版本；从未成功检查时为空串。
	LatestVersion string `json:"latest_version"`
	// ReleaseURL 是最新版本对应的页面地址。
	ReleaseURL string `json:"release_url"`
	// CheckedAt 是最近一次检查的时间（RFC3339）；从未检查过时为空串。
	CheckedAt string `json:"checked_at"`
	// Error 是最近一次检查失败的原因；成功时为空串。
	Error string `json:"error"`
	// UpdateAvailable 表示最新版本高于当前构建版本。
	UpdateAvailable bool `json:"update_available"`
}

// AboutInfo 是关于页展示的全部信息。
type AboutInfo struct {
	Version   string       `json:"version"`
	BuildTime string       `json:"build_time"`
	Update    UpdateStatus `json:"update"`
}

// AboutService 管理版本信息与更新检查状态。
//
// 检查结果只保存在内存中，进程重启后由当天的自动检查或手动检查重新获取。
type AboutService struct {
	DB        *gorm.DB
	Version   string
	BuildTime string
	// APIURL 是标签列表接口地址，测试可替换为本地服务。
	APIURL string
	// ReleasesURL 是没有标签可用时的兜底页面地址。
	ReleasesURL string
	// Client 是发起检查请求的客户端；为 nil 时使用默认 Transport（会读取 HTTPS_PROXY）。
	Client *http.Client

	mu            sync.Mutex
	inFlight      bool
	latestVersion string
	checkedAt     time.Time
	lastErr       string
}

// NewAboutService 构造服务。
func NewAboutService(db *gorm.DB, ver, buildTime string) *AboutService {
	return &AboutService{
		DB:          db,
		Version:     ver,
		BuildTime:   buildTime,
		APIURL:      repoAPIURL,
		ReleasesURL: repoReleasesURL,
	}
}

// Enabled 读取自动检查开关；没有落库时默认开启。
func (s *AboutService) Enabled() bool {
	// 查不到即为默认开启；数据库错误同样按开启处理，避免误静默
	enabled, err := (configStore{s.DB}).bool(KeyUpdateCheckEnabled, true)
	return err != nil || enabled
}

// SetEnabled 保存自动检查开关；开启时删除该 key，保持默认值语义。
func (s *AboutService) SetEnabled(enabled bool) error {
	if enabled {
		return (configStore{s.DB}).delete(KeyUpdateCheckEnabled)
	}
	return (configStore{s.DB}).set(KeyUpdateCheckEnabled, "false")
}

// Info 返回当前版本与上一次检查结果。
func (s *AboutService) Info() AboutInfo {
	// 开关要读库，先取出来再进锁，避免持有互斥锁做 I/O
	enabled := s.Enabled()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.infoLocked(enabled)
}

// infoLocked 组装 AboutInfo；调用方须持有 s.mu。
func (s *AboutService) infoLocked(enabled bool) AboutInfo {
	st := UpdateStatus{
		Enabled:       enabled,
		Comparable:    version.Valid(s.Version),
		LatestVersion: s.latestVersion,
		Error:         s.lastErr,
	}
	if st.LatestVersion != "" {
		st.ReleaseURL = repoTagURL + st.LatestVersion
		st.UpdateAvailable = version.IsNewer(st.LatestVersion, s.Version)
	} else {
		st.ReleaseURL = s.ReleasesURL
	}
	if !s.checkedAt.IsZero() {
		st.CheckedAt = s.checkedAt.UTC().Format(time.RFC3339)
	}
	return AboutInfo{Version: s.Version, BuildTime: s.BuildTime, Update: st}
}

// AutoCheck 在自动检查开启时执行一次检查；自动任务的入口。
//
// 开发构建（版本号非数字）不比较版本，也不出网。
func (s *AboutService) AutoCheck(ctx context.Context) {
	if !version.Valid(s.Version) {
		logger.FromContext(ctx).Debug("当前为开发构建，跳过自动检查更新", "version", s.Version)
		return
	}
	if !s.Enabled() {
		logger.FromContext(ctx).Debug("自动检查更新已关闭，跳过")
		return
	}
	s.CheckNow(ctx)
}

// CheckNow 立即检查一次更新；已有检查在运行时直接返回。
func (s *AboutService) CheckNow(ctx context.Context) {
	s.mu.Lock()
	if s.inFlight {
		s.mu.Unlock()
		return
	}
	s.inFlight = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.inFlight = false
		s.mu.Unlock()
	}()

	latest, releaseURL, err := s.fetchLatest(ctx)

	s.mu.Lock()
	s.checkedAt = time.Now()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
		s.latestVersion = latest
	}
	s.mu.Unlock()

	// 日志放在锁外：记录失败不需要阻塞读取状态的请求
	if err != nil {
		logger.FromContext(ctx).Warn("检查更新失败", "error", err)
		return
	}
	s.logSummary(ctx, latest, releaseURL)
}

// logSummary 记录一次成功的检查结论。
func (s *AboutService) logSummary(ctx context.Context, latest, releaseURL string) {
	l := logger.FromContext(ctx)
	if version.IsNewer(latest, s.Version) {
		l.Info("发现新版本", "current", s.Version, "latest", latest, "release_url", releaseURL)
		return
	}
	l.Debug("已是最新版本", "current", s.Version, "latest", latest)
}

// fetchLatest 拉取仓库标签，返回其中最大的版本号与对应页面地址。
func (s *AboutService) fetchLatest(ctx context.Context) (string, string, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: updateCheckTimeout}
	}
	url := fmt.Sprintf("%s?per_page=%d", s.APIURL, tagListLimit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "reminder-update-check")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("请求版本列表失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", fmt.Errorf("读取版本列表失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("版本列表接口返回 HTTP %d", resp.StatusCode)
	}

	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &tags); err != nil {
		return "", "", fmt.Errorf("解析版本列表失败: %w", err)
	}

	names := make([]string, 0, len(tags))
	for _, t := range tags {
		// 只保留可比较的版本标签，避免无关标签（如 nightly）占据「最新」位置
		if name := strings.TrimSpace(t.Name); version.Valid(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", "", errors.New("仓库没有可用的版本标签")
	}
	// 接口顺序不保证按版本号排列，取最大版本
	sort.Slice(names, func(i, j int) bool { return version.IsNewer(names[i], names[j]) })
	latest := names[0]
	return latest, repoTagURL + latest, nil
}
