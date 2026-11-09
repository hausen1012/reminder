package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newAboutTestService 构造指向本地假接口的关于服务。
func newAboutTestService(t *testing.T, version string) (*AboutService, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"v1.3.0"},{"name":"v1.5.2"},{"name":"nightly"}]`))
	}))
	t.Cleanup(srv.Close)

	svc := NewAboutService(openTestDB(t), version, "2026-11-01T08:00:00Z")
	svc.APIURL = srv.URL
	return svc, &calls
}

// 检查成功后应记录最新版本，并按版本号比较得出是否有更新。
func TestAboutService_CheckFindsNewerVersion(t *testing.T) {
	svc, calls := newAboutTestService(t, "v1.4.1")

	svc.CheckNow(context.Background())

	if *calls != 1 {
		t.Fatalf("接口调用次数 = %d，期望 1", *calls)
	}
	info := svc.Info()
	if info.Version != "v1.4.1" || info.BuildTime != "2026-11-01T08:00:00Z" {
		t.Fatalf("版本信息不符: %+v", info)
	}
	if info.Update.LatestVersion != "v1.5.2" {
		t.Fatalf("最新版本 = %q，期望 v1.5.2", info.Update.LatestVersion)
	}
	if !info.Update.UpdateAvailable {
		t.Fatal("应标记为有更新")
	}
	if info.Update.ReleaseURL == "" || info.Update.CheckedAt == "" {
		t.Fatalf("缺少发布地址或检查时间: %+v", info.Update)
	}
	if info.Update.Error != "" {
		t.Fatalf("成功时不应带错误: %q", info.Update.Error)
	}
}

// 当前版本已是最新时不应提示更新。
func TestAboutService_CheckUpToDate(t *testing.T) {
	svc, _ := newAboutTestService(t, "v1.5.2")
	svc.CheckNow(context.Background())

	info := svc.Info()
	if info.Update.UpdateAvailable {
		t.Fatalf("同版本不应提示更新: %+v", info.Update)
	}
	if info.Update.LatestVersion != "v1.5.2" {
		t.Fatalf("最新版本 = %q", info.Update.LatestVersion)
	}
}

// 接口失败时记录错误，且不覆盖上一次的成功结论。
func TestAboutService_CheckFailureKeepsPreviousResult(t *testing.T) {
	svc, _ := newAboutTestService(t, "v1.4.1")
	svc.CheckNow(context.Background())
	if svc.Info().Update.LatestVersion == "" {
		t.Fatal("前置检查应成功")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	svc.APIURL = srv.URL
	svc.CheckNow(context.Background())

	info := svc.Info()
	if info.Update.Error == "" {
		t.Fatal("失败时应记录 error")
	}
	if info.Update.LatestVersion != "v1.5.2" {
		t.Fatalf("失败不应清空上一次结论，实际 latest=%q", info.Update.LatestVersion)
	}
	if !info.Update.UpdateAvailable {
		t.Fatal("上一次结论为有更新，失败后仍应保持")
	}
}

// 关闭自动检查后 AutoCheck 不应出网。
func TestAboutService_AutoCheckRespectsSwitch(t *testing.T) {
	svc, calls := newAboutTestService(t, "v1.4.1")

	// 默认开启
	if !svc.Info().Update.Enabled {
		t.Fatal("默认应开启自动检查")
	}
	if err := svc.SetEnabled(false); err != nil {
		t.Fatalf("关闭开关失败: %v", err)
	}
	svc.AutoCheck(context.Background())
	if *calls != 0 {
		t.Fatalf("关闭后不应请求接口，实际调用 %d 次", *calls)
	}

	if err := svc.SetEnabled(true); err != nil {
		t.Fatalf("开启开关失败: %v", err)
	}
	svc.AutoCheck(context.Background())
	if *calls != 1 {
		t.Fatalf("开启后应请求一次，实际调用 %d 次", *calls)
	}
	if !svc.Info().Update.Enabled {
		t.Fatal("重新开启后开关应为开启")
	}
}

// 开发构建（版本号非数字）既不能比较版本，也不应自动出网；手动检查仍然可用。
func TestAboutService_DevBuildSkipsAutoCheck(t *testing.T) {
	svc, calls := newAboutTestService(t, "dev")

	if svc.Info().Update.Comparable {
		t.Fatal("dev 版本不应参与比较")
	}
	svc.AutoCheck(context.Background())
	if *calls != 0 {
		t.Fatalf("开发构建不应自动检查，实际调用 %d 次", *calls)
	}

	// 手动检查照常执行，但不会提示更新
	svc.CheckNow(context.Background())
	if *calls != 1 {
		t.Fatalf("手动检查应请求接口，实际调用 %d 次", *calls)
	}
	info := svc.Info()
	if info.Update.LatestVersion != "v1.5.2" {
		t.Fatalf("最新版本 = %q", info.Update.LatestVersion)
	}
	if info.Update.UpdateAvailable {
		t.Fatal("dev 版本不应提示更新")
	}
}
