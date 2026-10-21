// Package upass 实现同花顺统一登录（upass）的短信验证码登录流程。
//
// 背景：tzzb 站点只提供手机短信验证码登录，而登录页本身也受 chameleon 反爬
// 保护 —— 直连 https://upass.10jqka.com.cn/login 会拿到 401 + 一段加载
// chameleon 脚本后自我重定向的页面。带上后端自己算出的 hexin-v token 才能
// 拿到 200 的真实登录页（2026-10-22 实测）。
//
// 短信下发接口在未被风控时不需要图形验证码：登录页 JS 里原先"必须先过滑块"
// 的逻辑已被官方注释掉，改成"先调接口，看返回码决定是否弹验证"。返回码为 0
// 时短信直接下发；只有 -10507 / -11400 才要求滑块验证（纯 HTTP 无法通过，
// 应引导用户改走浏览器通道）。
//
// 除短信登录外，本包也实现了同花顺 APP 扫码登录（/scan/creatCode、/scan/creatImg、
// /scan/getInfoNew）：用户用 APP 扫二维码并确认即可拿到登录态，全程同样是纯 HTTP，
// 且不受图形验证码/滑块影响，是更稳的登录通道。
//
// 本包只负责"拿到登录态 Cookie"，Cookie 由调用方（cmd/login.go）交给 pkg/api 保存
// 并做一次真实接口校验。
package upass

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// baseURL 是同花顺统一登录站点。
	baseURL = "https://upass.10jqka.com.cn"
	// loginPath 是 tzzb 站内 iframe 登录用的地址（isIframe=1 决定登录页尺寸/功能）。
	loginPath = "/login?isIframe=1"
	// pubkeyPath 下发登录接口加密用的 RSA 公钥。
	pubkeyPath = "/pubkey/default.js"
	// captchaPath 下发登录用图形验证码（接口直接返回图片字节）。
	captchaPath = "/captcha/build"
	// sendSMSPath 下发短信验证码。
	//
	// 注意：旧版 /smslogin/sendSms 已被服务端改成"必须先过滑块验证"（-11400），
	// 纯 HTTP 无法通过；新版 /smsloginnew/send 改用图形验证码（captcha 参数），
	// 这是浏览器无关登录唯一可走通的路子。
	sendSMSPath = "/smsloginnew/send"
	// checkSMSPath 用短信验证码换登录态。
	checkSMSPath = "/smsloginnew/checkbyifind"

	// 扫码登录：申请 qrid / 下载二维码图片 / 轮询扫码状态。
	qrCreatePath = "/scan/creatCode"
	qrImagePath  = "/scan/creatImg"
	qrPollPath   = "/scan/getInfoNew"

	// loginReferer 必须是 tzzb 站点，直接访问 upass 会被风控/反爬区别对待。
	loginReferer = "https://tzzb.10jqka.com.cn/pc/index.html"
	// defaultUA 与 internal/browserread 里专用浏览器保持一致，降低指纹差异。
	defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.0.0 Safari/537.36"

	// 登录页 JS 里写死的表单常量（pageMain.init / sendPost）。
	sourceCode  = "pc_web" // 场景代号，默认值即 pc_web
	requestType = "login"
	ttype       = "WEB"
	// sdtis 是风控位标记，浏览器侧 bin_to_hex('110000100010') = "C22"。
	sdtis       = "C22"
	countryCode = "86"

	// qrPageSource 是 tzzb 在统一登录页里的场景代号（iframe 的 source 参数）。
	qrPageSource = "tzzb_web"
)

// 登录接口返回码（见登录页 main.js 的分支处理）。
const (
	CodeOK              = 0
	CodeRiskControl     = -10507 // 风控：滑动验证码
	CodeRiskControlAlt  = -11400 // 风控：滑动验证码（另一种触发）
	CodeRefused         = -10511 // 拒绝登录
	CodeTooManyAttempts = -11002 // 获取次数过多，24 小时后再试
	CodeCaptchaEmpty    = -11075 // 图形验证码不能为空
	CodeCaptchaWrong    = -11076 // 图形验证码输入错误
)

// HexinProvider 返回一个新的 hexin-v 反爬 token。为 nil 表示不添加该请求头。
type HexinProvider func() (string, error)

// Option 定制 Client。
type Option func(*Client)

// WithHexinProvider 设置 hexin-v token 提供者（通常来自 internal/hexinv）。
func WithHexinProvider(f HexinProvider) Option {
	return func(c *Client) { c.hexin = f }
}

// WithHTTPClient 注入自定义 HTTP 客户端（测试用；不会自动带 Cookie jar）。
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.hc = hc }
}

// WithBaseURL 覆盖站点地址（测试用）。
func WithBaseURL(u string) Option {
	return func(c *Client) { c.base = strings.TrimRight(u, "/") }
}

// WithDebugWriter 打开调试日志，把请求/响应摘要写到 w。
func WithDebugWriter(w io.Writer) Option {
	return func(c *Client) { c.debug = w }
}

// Client 是同花顺登录会话。同一个 Client 内 Cookie jar 共享，
// 因此"发短信"和"提交验证码"必须在同一个 Client 上完成。
type Client struct {
	hc    *http.Client
	base  string
	hexin HexinProvider
	debug io.Writer

	mu     sync.Mutex
	pub    *pubKey
	pageOK bool
}

// New 创建登录客户端。
func New(opts ...Option) *Client {
	jar, _ := cookiejar.New(nil)
	c := &Client{
		hc:   &http.Client{Jar: jar, Timeout: 20 * time.Second},
		base: baseURL,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// do 给每个请求补齐浏览器同款请求头，并附加新的 hexin-v token。
func (c *Client) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", defaultUA)
	req.Header.Set("Referer", loginReferer)
	req.Header.Set("Origin", "https://upass.10jqka.com.cn")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if c.hexin != nil {
		tok, err := c.hexin()
		if err != nil {
			return nil, fmt.Errorf("生成 hexin-v 反爬 token 失败: %w", err)
		}
		req.Header.Set("hexin-v", tok)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if c.debug != nil {
		fmt.Fprintf(c.debug, "[upass] %s %s -> HTTP %d\n", req.Method, req.URL.Path, resp.StatusCode)
	}
	return resp, nil
}

// ensureSession 打开登录页（拿 PHPSESSID）并拉取 RSA 公钥，只做一次。
func (c *Client) ensureSession(ctx context.Context) error {
	if err := c.ensureLoginPage(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	haveKey := c.pub != nil
	c.mu.Unlock()
	if haveKey {
		return nil
	}

	k, err := c.fetchPubKey(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.pub = k
	c.mu.Unlock()
	return nil
}

// ensureLoginPage 打开真实登录页。没有有效 hexin-v 时站点会返回 401 中转页，
// 这里转成可读错误，提示检查反爬 token 是否可用。
func (c *Client) ensureLoginPage(ctx context.Context) error {
	c.mu.Lock()
	done := c.pageOK
	c.mu.Unlock()
	if done {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+loginPath, nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("打开同花顺登录页失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if c.debug != nil {
		fmt.Fprintf(c.debug, "[upass] 登录页 HTTP %d，%d 字节\n", resp.StatusCode, len(body))
	}

	switch resp.StatusCode {
	case http.StatusOK:
		c.mu.Lock()
		c.pageOK = true
		c.mu.Unlock()
		return nil
	case http.StatusUnauthorized:
		return errors.New("同花顺登录页被反爬拦截（HTTP 401）：hexin-v token 无效，或已用 TZZB_NO_HEXIN=1 关闭")
	default:
		return fmt.Errorf("打开同花顺登录页失败: HTTP %d（%s）", resp.StatusCode, truncate(string(body), 160))
	}
}

// CaptchaImage 获取登录用图形验证码图片（JPEG 字节）。
//
// 图片与该 Client 的 PHPSESSID 绑定，所以必须用同一个 Client 提交识别结果。
func (c *Client) CaptchaImage(ctx context.Context) ([]byte, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	u := c.base + captchaPath + "?t=" + strconv.FormatInt(time.Now().UnixMilli(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("获取图形验证码失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取图形验证码失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取图形验证码失败: HTTP %d（%s）", resp.StatusCode, truncate(string(body), 120))
	}
	if len(body) == 0 {
		return nil, errors.New("图形验证码接口返回空内容")
	}
	return body, nil
}

// SendSMS 用图形验证码换取短信下发，返回接口给的 signcode token（登录时可能需要）。
//
// 图形验证码识别错误时返回 *APIError 且 IsCaptchaWrong() 为真 —— 调用方应换一张
// 新图重试。短信真正下发成功时返回 nil 错误。
func (c *Client) SendSMS(ctx context.Context, phone, captcha string) (string, error) {
	phone = NormalizePhone(phone)
	if !ValidPhone(phone) {
		return "", fmt.Errorf("手机号格式不正确：%q（应为 11 位大陆手机号）", phone)
	}
	captcha = strings.TrimSpace(captcha)
	if captcha == "" {
		return "", errors.New("图形验证码不能为空")
	}
	if err := c.ensureSession(ctx); err != nil {
		return "", err
	}

	c.mu.Lock()
	pub := c.pub
	c.mu.Unlock()
	enc, err := pub.encryptHex(phone)
	if err != nil {
		return "", err
	}

	form := c.baseForm(pub)
	form.Set("mobile", enc)
	form.Set("captcha", captcha)
	resp, err := c.postForm(ctx, sendSMSPath, form)
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

// LoginSMS 提交短信验证码换取登录态，返回可直接写入 ~/.tzzb/cookies 的 Cookie 串。
//
// 必须与 SendSMS 使用同一个 Client（复用 PHPSESSID 与 signcode token）。
func (c *Client) LoginSMS(ctx context.Context, phone, code, signToken string) (string, error) {
	phone = NormalizePhone(phone)
	code = strings.TrimSpace(code)
	if !ValidPhone(phone) {
		return "", fmt.Errorf("手机号格式不正确：%q（应为 11 位大陆手机号）", phone)
	}
	if !validSMSCode(code) {
		return "", fmt.Errorf("短信验证码格式不正确：%q（应为 6 位数字）", code)
	}
	if err := c.ensureSession(ctx); err != nil {
		return "", err
	}

	c.mu.Lock()
	pub := c.pub
	c.mu.Unlock()
	encPhone, err := pub.encryptHex(phone)
	if err != nil {
		return "", err
	}
	encCode, err := pub.encryptHex(code)
	if err != nil {
		return "", err
	}

	form := c.baseForm(pub)
	form.Set("mobile", encPhone)
	form.Set("signcode", encCode)
	form.Set("longLogin", "0")
	if signToken != "" {
		form.Set("token", signToken)
	}
	if _, err := c.postForm(ctx, checkSMSPath, form); err != nil {
		return "", err
	}

	cookie := c.cookieString()
	if cookie == "" {
		return "", errors.New("登录接口返回成功，但没有拿到任何 Cookie（接口可能已变更）")
	}
	return cookie, nil
}

// 扫码登录轮询状态。
const (
	QRStatusExpired   = 0 // 二维码失效，需要重新获取
	QRStatusWaiting   = 1 // 等待用户用同花顺 APP 扫码
	QRStatusScanned   = 2 // 已扫码，等待用户在手机上确认
	QRStatusConfirmed = 3 // 确认成功，会话已登录
)

// QRCode 是一张登录二维码。
type QRCode struct {
	QRID  string
	Image []byte // PNG
}

// CreateQRCode 申请一张扫码登录二维码，返回 qrid 与二维码 PNG。
//
// 扫码登录全程纯 HTTP：用户用同花顺 APP 扫描二维码并确认，服务端在轮询响应里
// 下发登录态 Cookie，不需要图形验证码/滑块/短信。
func (c *Client) CreateQRCode(ctx context.Context) (*QRCode, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	raw, status, err := c.getBytes(ctx, qrCreatePath)
	if err != nil {
		return nil, fmt.Errorf("获取登录二维码失败: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("获取登录二维码失败: HTTP %d（%s）", status, truncate(string(raw), 120))
	}
	var r struct {
		QRID      string `json:"qrid"`
		ErrorCode int    `json:"errorCode"`
		ErrorMsg  string `json:"errorMsg"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("二维码接口返回的不是 JSON：%s", truncate(string(raw), 160))
	}
	if r.ErrorCode != CodeOK || r.QRID == "" {
		return nil, &APIError{Code: r.ErrorCode, Msg: r.ErrorMsg}
	}

	img, status, err := c.getBytes(ctx, qrImagePath+"?qrid="+url.QueryEscape(r.QRID)+"&source="+qrSource())
	if err != nil {
		return nil, fmt.Errorf("下载二维码图片失败: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("下载二维码图片失败: HTTP %d", status)
	}
	if len(img) == 0 {
		return nil, errors.New("二维码图片为空")
	}
	return &QRCode{QRID: r.QRID, Image: img}, nil
}

// qrSource 返回二维码的场景代号：默认跟随 tzzb 登录页（tzzb_web），
// 可用 TZZB_QR_SOURCE 覆盖（例如 pc_web 会编码成手机端扫码登录地址，
// 便于对比 APP 扫码行为；两者 qrid 状态机相同）。
func qrSource() string {
	if v := strings.TrimSpace(os.Getenv("TZZB_QR_SOURCE")); v != "" {
		return v
	}
	return qrPageSource
}

// PollQRCode 轮询扫码状态，返回 QRStatus* 之一。
func (c *Client) PollQRCode(ctx context.Context, qrid string) (int, error) {
	form := url.Values{}
	form.Set("qrid", qrid)
	form.Set("state", "1")
	form.Set("source", "pc_web")
	form.Set("page_source", "web_screen")
	form.Set("request_type", requestType)
	raw, status, err := c.postRaw(ctx, qrPollPath, form)
	if err != nil {
		return 0, fmt.Errorf("轮询扫码状态失败: %w", err)
	}
	if status != http.StatusOK {
		return 0, fmt.Errorf("轮询扫码状态失败: HTTP %d（%s）", status, truncate(string(raw), 120))
	}
	var r struct {
		Status    int    `json:"status"`
		Expired   int    `json:"expired"` // 注意：正常等待时也可能为 1，不可据此判失效
		ErrorCode int    `json:"errorCode"`
		ErrorMsg  string `json:"errorMsg"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, fmt.Errorf("扫码状态接口返回的不是 JSON：%s", truncate(string(raw), 160))
	}
	if r.ErrorCode != CodeOK && r.ErrorCode != 0 {
		return 0, &APIError{Code: r.ErrorCode, Msg: r.ErrorMsg}
	}
	// 失效以 status=0 为准（页面 JS 里 case 0 才刷新二维码）。
	return r.Status, nil
}

// Cookies 返回当前会话里可直接写入 Cookie 文件的 Cookie 串。
func (c *Client) Cookies() string { return c.cookieString() }

// getBytes 发送 GET 请求并读回响应体与状态码。
func (c *Client) getBytes(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("请求 %s 失败: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

// postRaw 发送表单 POST 并读回原始响应体与状态码。
func (c *Client) postRaw(ctx context.Context, path string, form url.Values) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	resp, err := c.do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("请求 %s 失败: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, 0, err
	}
	return body, resp.StatusCode, nil
}

// baseForm 构造两个登录接口共用的表单字段（对应浏览器 sendPost 里补的公共参数）。
func (c *Client) baseForm(pub *pubKey) url.Values {
	f := url.Values{}
	f.Set("rsa_version", pub.Version)
	f.Set("request_type", requestType)
	f.Set("country_code", countryCode)
	f.Set("source", sourceCode)
	f.Set("ttype", ttype)
	f.Set("sdtis", sdtis)
	f.Set("timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	// 滑块验证参数：未触发风控时为空。浏览器用 jQuery 序列化时 undefined 会变成
	// 空字符串，这里显式带上，尽量与真实请求一致。
	f.Set("captcha_type", "")
	f.Set("captcha_phrase", "")
	f.Set("captcha_ticket", "")
	f.Set("captcha_signature", "")
	return f
}

// postForm 发送表单并解析登录接口的 JSON 返回。
func (c *Client) postForm(ctx context.Context, path string, form url.Values) (*apiResp, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", c.base+loginPath)

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("请求同花顺登录接口失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取登录接口响应失败: %w", err)
	}
	if c.debug != nil {
		fmt.Fprintf(c.debug, "[upass] POST %s -> HTTP %d\n%s\n", path, resp.StatusCode, truncate(string(body), 600))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("登录接口返回 HTTP %d：%s", resp.StatusCode, truncate(string(body), 200))
	}

	var r apiResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("登录接口返回的不是 JSON（可能被反爬拦成中转页）：%s", truncate(string(body), 200))
	}
	if r.ErrorCode != CodeOK {
		return nil, &APIError{Code: r.ErrorCode, Msg: r.ErrorMsg, DisposeID: r.DisposeID}
	}
	return &r, nil
}

// cookieString 汇总 Cookie jar 里 10jqka 各子域的 Cookie。
func (c *Client) cookieString() string {
	if c.hc.Jar == nil {
		return ""
	}
	urls := []string{
		"https://upass.10jqka.com.cn/",
		"https://tzzb.10jqka.com.cn/",
		"https://www.10jqka.com.cn/",
	}
	seen := map[string]string{}
	var order []string
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, ck := range c.hc.Jar.Cookies(u) {
			if _, ok := seen[ck.Name]; ok {
				continue
			}
			seen[ck.Name] = ck.Value
			order = append(order, ck.Name)
		}
	}
	parts := make([]string, 0, len(order))
	for _, n := range order {
		parts = append(parts, n+"="+seen[n])
	}
	return strings.Join(parts, "; ")
}

// CookieNames 返回当前 jar 里的 Cookie 名（调试用，不含值）。
func (c *Client) CookieNames() []string {
	names := map[string]bool{}
	var out []string
	if c.hc.Jar == nil {
		return nil
	}
	for _, raw := range []string{"https://upass.10jqka.com.cn/", "https://tzzb.10jqka.com.cn/"} {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		for _, ck := range c.hc.Jar.Cookies(u) {
			if !names[ck.Name] {
				names[ck.Name] = true
				out = append(out, ck.Name)
			}
		}
	}
	return out
}

// apiResp 是登录接口的 JSON 返回体。
type apiResp struct {
	ErrorCode int    `json:"errorcode"`
	ErrorMsg  string `json:"errormsg"`
	DisposeID string `json:"disposeid"`
	// Token 是短信下发成功后返回的 signcode token，提交验证码时可能需要。
	Token string `json:"token"`
}

// APIError 表示登录接口明确返回的业务错误。
type APIError struct {
	Code      int
	Msg       string
	DisposeID string
}

func (e *APIError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("同花顺登录接口返回错误 %d：%s", e.Code, e.Msg)
	}
	return fmt.Sprintf("同花顺登录接口返回错误 %d", e.Code)
}

// IsRiskControl 表示触发了滑块验证风控，纯 HTTP 无法继续。
func (e *APIError) IsRiskControl() bool {
	return e.Code == CodeRiskControl || e.Code == CodeRiskControlAlt
}

// IsCaptchaWrong 表示图形验证码识别错误，需要换一张重试。
func (e *APIError) IsCaptchaWrong() bool {
	return e.Code == CodeCaptchaWrong || e.Code == CodeCaptchaEmpty
}

// IsTooMany 表示短信获取次数超过当日限制。
func (e *APIError) IsTooMany() bool { return e.Code == CodeTooManyAttempts }

// IsRefused 表示服务端明确拒绝本次登录。
func (e *APIError) IsRefused() bool { return e.Code == CodeRefused }

// Hint 返回给用户看的处理建议。
func (e *APIError) Hint() string {
	switch {
	case e.IsCaptchaWrong():
		return "图形验证码不正确，请换一张重试"
	case e.IsRiskControl():
		return "触发同花顺风控（滑动验证码），请改用浏览器通道：tzzb browser login"
	case e.IsTooMany():
		return "短信获取次数过多，同花顺限制 24 小时后才能重试"
	case e.IsRefused():
		return "同花顺拒绝了本次登录请求（账号可能被临时限制），请稍后重试，或改用浏览器登录"
	default:
		return "可稍后重试；若持续失败，请改用浏览器通道：tzzb browser login"
	}
}

// NormalizePhone 清洗手机号：去空格/横线、去掉 +86 / 0086 前缀。
func NormalizePhone(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimPrefix(s, "+86")
	s = strings.TrimPrefix(s, "0086")
	return s
}

// ValidPhone 判断是否 11 位大陆手机号。
func ValidPhone(s string) bool {
	if len(s) != 11 || s[0] != '1' || s[1] < '3' || s[1] > '9' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// validSMSCode 判断是否 6 位数字验证码。
func validSMSCode(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// truncate 截断过长文本（按 rune，避免切坏 UTF-8），用于错误提示。
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
