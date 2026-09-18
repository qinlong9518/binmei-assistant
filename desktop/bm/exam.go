package bm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ExamPage 答题页解析结果
type ExamPage struct {
	KSMXID       string   // 考卷明细 ID
	ExamName     string
	ExamDuration string
	Code         string   // 站点代码（0018 → OnlineTestOne）
	Questions    []Question
}

// Question 单题
type Question struct {
	// 原始 17 字段（提交时取前 15）
	Fields []string
	// 解析出的语义字段
	Answer   string // 字段[13] 正确答案（单选字母 / 判断 Y/N）
	Style    string // 字段[14] 1=单选 3=判断
}

// FetchExamPage 拉取并解析答题页（需先 SetOnlineTest）
func (c *Client) FetchExamPage() (*ExamPage, error) {
	page, err := c.HTTPGetRaw("/P_ExamDetail/OnlineTestOne")
	if err != nil {
		return nil, err
	}
	if strings.Contains(page, "Object moved") {
		return nil, errors.New("答题会话失效")
	}
	v, err := extractVData(page)
	if err != nil {
		return nil, fmt.Errorf("vData 解析失败: %w", err)
	}
	if v.ErrorMsg != "" {
		return nil, fmt.Errorf("站点错误: %s", v.ErrorMsg)
	}
	ep := &ExamPage{
		KSMXID: v.KSMXID, ExamName: v.ExamName,
		ExamDuration: v.ExamDuration, Code: v.Code,
	}
	for _, row := range strings.Split(v.AllQuestionArray, "|") {
		if row == "" {
			continue
		}
		f := strings.Split(row, ",")
		if len(f) < 15 {
			continue
		}
		q := Question{Fields: f}
		if len(f) > 13 {
			q.Answer = f[13]
		}
		if len(f) > 14 {
			q.Style = f[14]
		}
		ep.Questions = append(ep.Questions, q)
	}
	return ep, nil
}

// vdData 答题页内联数据
type vdData struct {
	ErrorMsg       string `json:"ErrorMsg"`
	KSMXID         string `json:"ksmxid"`
	ExamName       string `json:"ExamName"`
	ExamDuration   string `json:"ExamDuration"`
	Code           string `json:"Code"`
	AllQuestionArray string `json:"AllQuestionArray"`
	AllCount         json.Number `json:"AllCount"`
	AllQuestionType  string `json:"AllQuestionType"`
}

// extractVData 从 HTML 提取 var vData={...}（括号计数 + 字符串感知）
func extractVData(html string) (*vdData, error) {
	i := strings.Index(html, "var vData=")
	if i < 0 {
		return nil, errors.New("未找到 vData")
	}
	j := strings.Index(html[i:], "{") + i
	depth, instr, esc := 0, false, false
	end := -1
	for k := j; k < len(html); k++ {
		ch := html[k]
		if esc {
			esc = false
			continue
		}
		switch ch {
		case '\\':
			if instr {
				esc = true
			}
		case '"':
			instr = !instr
		case '{':
			if !instr {
				depth++
			}
		case '}':
			if !instr {
				depth--
				if depth == 0 {
					end = k + 1
				}
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		return nil, errors.New("vData 括号未闭合")
	}
	var v vdData
	if err := json.Unmarshal([]byte(html[j:end]), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// BuildAnswerString 生成提交报文：每题字段[4]填答案，拼前 15 字段，题间 '|'
// （与站点 SaveDA 完全一致；判断题对错按站点逻辑：'对'→'Y'，'错'→'N'，空值不转）
func BuildAnswerString(ep *ExamPage) string {
	parts := make([]string, 0, len(ep.Questions))
	for _, q := range ep.Questions {
		f := make([]string, len(q.Fields))
		copy(f, q.Fields)
		if len(f) > 4 {
			// 判断题：站点 SetDa 语义 da=='对' → checked；写回时 '对'→'Y'/'错'→'N'
			ans := q.Answer
			if q.Style == "3" && ans != "" {
				// 站点提交端直接用 Y/N（字段[13] 本身就是 Y/N，无需转换）
			}
			f[4] = ans
		}
		// 清理字段[12]我的答案（站点提交保留原值，这里保守不动）
		if len(f) > 15 {
			f = f[:15] // 只拼前 15 字段（SaveDA 注释掉了 15、16）
		}
		parts = append(parts, strings.Join(f, ","))
	}
	return strings.Join(parts, "|")
}

// SubmitAnswers 提交全部答案
func (c *Client) SubmitAnswers(ksmxid, answer string) error {
	form := url.Values{}
	form.Set("answer", answer)
	form.Set("ksmxid", ksmxid)
	_, err := c.postForm("/Home/DoWriteAnswerAllOne", form)
	return err
}

// EndExam 交卷算分（返回 "分数|...|合格/不合格" 原文）
func (c *Client) EndExam(ksmxid string) (string, error) {
	form := url.Values{}
	form.Set("ksmxid", ksmxid)
	return c.postForm("/Home/EndTimeOne", form)
}

// RunExamMode 完整答题流程（examType: "2"=手机考试 / "1"=模拟考试，决定积分归属）
func (c *Client) RunExamMode(paper *Paper, examType string) (score string, err error) {
	ksmxid, err := c.CreateTempExamMode(paper.PaperID, examType)
	if err != nil {
		return "", err
	}
	if err := c.SetOnlineTest(ksmxid); err != nil {
		return "", err
	}
	ep, err := c.FetchExamPage()
	if err != nil {
		return "", err
	}
	if err := c.SubmitAnswers(ksmxid, BuildAnswerString(ep)); err != nil {
		return "", err
	}
	res, err := c.EndExam(ksmxid)
	if err != nil {
		return "", err
	}
	return res, nil
}