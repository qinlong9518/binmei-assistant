package bm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Client 彬煤站点协议客户端（每个 Client 独立 CookieJar = 独立登录会话，多账号互不干扰）
type Client struct {
	HTTP    *http.Client
	Base    string
	Account string
	PID     string
	Name    string
	Pwd     string

	// FirstIndexOne 解析出的会话上下文
	ExamTypeID      string // 当前考试类型（手机考试/模拟考试所属）
	DepartmentID    string
	ExamTypeName    string
}

const DefaultPassword = "Ydmk12345.6"

// MobileUA 与 App 端一致的 UA
const MobileUA = "Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.77 Mobile Safari/537.36"

// ErrNeedCaptcha 登录被要求验证码
var ErrNeedCaptcha = errors.New("站点要求验证码")

// NewClient 创建客户端
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		HTTP: &http.Client{
			Timeout: 25 * time.Second,
			Jar:     jar,
		},
		Base: "http://61.185.41.209:8888",
	}
}

// get / post 基础方法（自动带 UA + XHR 头）
func (c *Client) get(path string) (string, error) {
	req, err := http.NewRequest("GET", c.Base+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", MobileUA)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) postForm(path string, form url.Values) (string, error) {
	req, err := http.NewRequest("POST", c.Base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", MobileUA)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Login 登录：先 yzm=1，失败回退 yzm=auto（与 App 端 BmHttpLogin 相同的双保险）
func (c *Client) Login(account, password string) error {
	if err := c.tryLogin(account, password, "1"); err != nil {
		if errors.Is(err, ErrNeedCaptcha) {
			log.Printf("[login] yzm=1 被要求验证码，回退 yzm=auto")
		} else {
			return err
		}
		if err := c.tryLogin(account, password, "auto"); err != nil {
			return err
		}
	}
	c.Account = account
	c.Pwd = password
	// 登录后立刻建立服务端会话（写 Session 的登录标记）
	if _, err := c.InitSession(); err != nil {
		return fmt.Errorf("建立会话失败: %w", err)
	}
	return nil
}

// tryLogin 单次登录尝试
func (c *Client) tryLogin(account, password, yzm string) error {
	form := url.Values{}
	form.Set("idcard", FormEscape(account))
	form.Set("openid", "")
	form.Set("yzm", yzm)
	form.Set("pwd", FormEscape(password))
	form.Set("style", "0")
	form.Set("auto", "true")

	body, err := c.postForm("/PersonWap/GetPersonInfo", form)
	if err != nil {
		return err
	}
	// 响应：成功 "|pid|姓名|md5"；失败 "错误信息|..."
	strs := strings.Split(body, "|")
	if strs[0] == "" {
		if len(strs) < 2 {
			return fmt.Errorf("登录响应异常: %q", body)
		}
		c.PID = strs[1]
		if len(strs) > 2 {
			c.Name = strings.TrimSpace(strs[2])
		}
		return nil
	}
	msg := strs[0]
	if strings.Contains(msg, "验证码") {
		return fmt.Errorf("%w: %s", ErrNeedCaptcha, msg)
	}
	return errors.New(msg)
}

// InitSession 建立/刷新服务端会话（等价站点首页 ChangeIndexJF：POST FirstIndexOne，原始 pid）
// 返回响应原文（'|' 分割，含考试类型等上下文）
func (c *Client) InitSession() (string, error) {
	if c.PID == "" {
		return "", errors.New("未登录")
	}
	form := url.Values{}
	form.Set("pid", c.PID)
	form.Set("wx", "")
	body, err := c.postForm("/PersonWap/FirstIndexOne", form)
	if err != nil {
		return "", err
	}
	if strings.Contains(body, "Object moved") || strings.Contains(body, "LoginWap2") {
		return "", errors.New("会话未建立（服务端拒绝）")
	}
	strs := strings.Split(body, "|")
	// 实测: |0|0018|部门id|类型名||IsCog|考试类型id|52|9|82.7%||姓名|照片路径|
	if len(strs) > 6 {
		c.ExamTypeID = strings.TrimSpace(strs[6])
	}
	if len(strs) > 3 {
		c.DepartmentID = strings.TrimSpace(strs[3])
	}
	if len(strs) > 4 {
		c.ExamTypeName = strings.TrimSpace(strs[4])
	}
	return body, nil
}

// ---------- 积分 ----------

// PointsDetail 积分明细项（对外序列化用短名）
type PointsDetail struct {
	Name string  `json:"Name"`
	Cur  float64 `json:"Cur"`
	Max  float64 `json:"Max"`
}

// sitePoints 站点原始响应结构
type sitePoints struct {
	AccumulateName string  `json:"AccumulateName"`
	CurAccumulate  float64 `json:"CurAccumulate"`
	MaxAccumulate  float64 `json:"MaxAccumulate"`
}

type pointsResp struct {
	Success bool         `json:"success"`
	Data    []sitePoints `json:"data"`
	Message string       `json:"message"`
}

// GetPoints 今日积分明细（参数为 Esdt(pid)，与 App 积分监控一致）
func (c *Client) GetPoints() ([]PointsDetail, error) {
	if c.PID == "" {
		return nil, errors.New("未登录")
	}
	u := fmt.Sprintf("/AccumulateManger/S_Accumulate/GetPersonTodayAccumulateOne?pid=%s", QueryEscapePid(c.PID))
	body, err := c.get(u)
	if err != nil {
		return nil, err
	}
	var r pointsResp
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return nil, fmt.Errorf("积分响应解析失败: %w", err)
	}
	if !r.Success {
		return nil, fmt.Errorf("积分接口失败: %s", r.Message)
	}
	out := make([]PointsDetail, 0, len(r.Data))
	for _, p := range r.Data {
		out = append(out, PointsDetail{Name: p.AccumulateName, Cur: p.CurAccumulate, Max: p.MaxAccumulate})
	}
	return out, nil
}

// ---------- 考试 ----------

// Paper 试卷条目
type Paper struct {
	PaperID      string  `json:"PaperId"`
	PaperName    string  `json:"PaperName"`
	TotalScore   float64 `json:"TotalScore"`
	PassingGrade float64 `json:"PassingGrade"`
}

type listResp struct {
	Success bool    `json:"success"`
	Data    []Paper `json:"data"`
}

// SelectCanRunList 拉取某考试类型的可考试卷列表
func (c *Client) SelectCanRunList(examTypeID string) ([]Paper, error) {
	if examTypeID == "" {
		// 会话上下文缺失时重建
		if _, err := c.InitSession(); err != nil {
			return nil, err
		}
	}
	u := fmt.Sprintf("/ExamManger/P_Paper/SelectCanRunListOne?rows=20&page=1&examtypeid=%s&pid=%s",
		url.QueryEscape(examTypeID), url.QueryEscape(c.PID))
	body, err := c.get(u)
	if err != nil {
		return nil, err
	}
	var r listResp
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return nil, fmt.Errorf("试卷列表解析失败: %w", err)
	}
	if !r.Success {
		return nil, errors.New("试卷列表接口失败")
	}
	return r.Data, nil
}

// ExistsNoFinish 检查试卷是否有未完成的考试（0=无）
func (c *Client) ExistsNoFinish(paperID string) (string, error) {
	form := url.Values{}
	form.Set("PaperId", paperID)
	form.Set("pid", c.PID)
	form.Set("type", "1")
	body, err := c.postForm("/ExamManger/P_ExamDetail/ExistsNoFinish", form)
	if err != nil {
		return "", err
	}
	var r struct {
		Success bool        `json:"success"`
		Data    interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return "", err
	}
	if !r.Success {
		return "", errors.New("ExistsNoFinish 失败")
	}
	return fmt.Sprintf("%v", r.Data), nil
}

// HTTPGetRaw 普通页面 GET（不带 XHR 头，模拟浏览器整页跳转）
func (c *Client) HTTPGetRaw(path string) (string, error) {
	req, err := http.NewRequest("GET", c.Base+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", MobileUA)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SetOnlineTest 写入答题会话（与站点 OpenTest 一致：全参数 Esdt 编码）
func (c *Client) SetOnlineTest(ksmxid string) error {
	form := url.Values{}
	form.Set("ksmxid", FormEscape(ksmxid))
	form.Set("showBZDA", FormEscape("0"))
	form.Set("style", FormEscape("0"))
	form.Set("vErr", FormEscape("-1"))
	form.Set("nc", FormEscape(""))
	form.Set("r", FormEscape(""))
	form.Set("e", FormEscape(""))
	form.Set("b", FormEscape(""))
	form.Set("pid", FormEscape(c.PID))
	_, err := c.postForm("/P_ExamDetail/SetOnlineTestOne", form)
	return err
}

// CreateTempExamMode 建考卷（返回 ksmxid）
// examType: "2"=手机考试（MobileExam 链路）、"1"=模拟考试（SelectPaper 链路）
// type 决定积分归属（实测：type=2 加手机考试分、type=1 加模拟考试分）
func (c *Client) CreateTempExamMode(paperID, examType string) (string, error) {
	form := url.Values{}
	form.Set("PaperId", paperID)
	form.Set("PersonId", c.PID)
	form.Set("IDNumber", "")
	form.Set("type", examType)
	body, err := c.postForm("/Home/CreateTempExamOne", form)
	if err != nil {
		return "", err
	}
	var r struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return "", fmt.Errorf("建卷响应解析失败: %w", err)
	}
	if !r.Success {
		return "", fmt.Errorf("建卷失败: %s", r.Message)
	}
	return r.Data, nil
}

// CreateTempExam 建考卷（默认手机考试 type=2）
func (c *Client) CreateTempExam(paperID string) (string, error) {
	form := url.Values{}
	form.Set("PaperId", paperID)
	form.Set("PersonId", c.PID)
	form.Set("IDNumber", "")
	form.Set("type", "2")
	body, err := c.postForm("/Home/CreateTempExamOne", form)
	if err != nil {
		return "", err
	}
	var r struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		return "", fmt.Errorf("建卷响应解析失败: %w", err)
	}
	if !r.Success {
		return "", fmt.Errorf("建卷失败: %s", r.Message)
	}
	return r.Data, nil
}
