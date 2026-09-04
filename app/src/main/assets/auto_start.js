// 自动开考调度脚本 v3（状态机 + 变化检测驱动，不再依赖固定冷却）
// 注入主 WebView，每 3s 轮询：
//   1) 读取当前状态快照（页面标题 / 考试列表 / 积分明细 / 考试页特征）
//   2) 状态与上一轮相同 → 跳过；有变化 → 立即执行下一步动作
//   3) 动作失败或无进展时按状态机重新决策，不会死等
// 目标：手机考试/模拟考试积分未满 → 自动进对应列表 → 点【试卷二】→ 答题引擎接管
// 门控：window.BM_AUTOSTART_DISABLED=true 时完全停用（进入软件的询问弹窗「暂不自动」）
(function() {
    'use strict';

    if (window.self !== window.top) return;

    if (window.BM_AUTOSTART_INJECTED) {
        console.log("[自动开考] 已注入，跳过");
        return;
    }
    window.BM_AUTOSTART_INJECTED = true;

    var POLL_MS = getPollMs();   // 轮询间隔（设置页可调，bm-cfg 热更新生效）
    var TAB_TRIES_LIMIT = 4; // 同一 Tab 连点上限（防异常循环）

    var lastSig = "";        // 上一轮状态签名
    var tabTries = 0;
    var lastTabId = "";

    function log(m) { console.log("[自动开考] " + m); }

    function isDisabled() {
        try { return window.BM_AUTOSTART_DISABLED === true; } catch (e) { return false; }
    }

    // 页面标题（考试列表片段有 mui-title；壳内首页没有）
    function pageTitle() {
        var t = document.querySelector('.mui-bar-nav .mui-title');
        return t ? (t.textContent || "").trim() : "";
    }

    function examKeyForPage() {
        var t = pageTitle();
        if (t.indexOf("手机考试") !== -1) return "手机考试";
        if (t.indexOf("模拟考试") !== -1) return "模拟考试";
        return "";
    }

    // 当前需要补分的考试：手机考试优先 → 模拟考试；null=全满/数据未就绪
    function pickTarget() {
        try {
            var d = window.BM_EXAM_POINTS;
            if (!d || typeof d !== "object") return null;
            var m = d["手机考试"], s = d["模拟考试"];
            var mFull = !(m && typeof m.cur === "number") || m.cur >= (m.max || 0);
            var sFull = !(s && typeof s.cur === "number") || s.cur >= (s.max || 0);
            if (!mFull) return "手机考试";
            if (!sFull) return "模拟考试";
            return null;
        } catch (e) { return null; }
    }

    // 该考试积分是否已满（数据缺失视为已满=不操作）
    function examFull(key) {
        try {
            var e = (window.BM_EXAM_POINTS || {})[key];
            if (!e || typeof e.cur !== "number") return true;
            return e.cur >= (e.max || 0);
        } catch (err) { return true; }
    }

    function inExamPage() {
        try {
            return !!(window.vData && window.onlineCur && window.allShiTi);
        } catch (e) { return false; }
    }

    // 状态签名：任意要素变化即视为"页面有变化"
    function snapshot() {
        var lis = document.querySelectorAll('#canRunExamList > li.mui-table-view-cell');
        var names = [];
        for (var i = 0; i < lis.length; i++) names.push((lis[i].innerText || "").replace(/\s/g, ""));
        try {
            return [
                pageTitle(),
                inExamPage() ? ('exam:' + (window.onlineCur || '')) : 'page',
                names.join('|'),
                JSON.stringify(window.BM_EXAM_POINTS || {})
            ].join('#');
        } catch (e) { return String(Date.now()); }
    }

    function fireTap(el) {
        if (!el) return;
        try {
            el.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true }));
            el.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true }));
        } catch (e) {}
        try { el.dispatchEvent(new Event('tap', { bubbles: true })); } catch (e) {}
        try { el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })); } catch (e) {}
        try { el.dispatchEvent(new MouseEvent('mouseup', { bubbles: true })); } catch (e) {}
        try { el.dispatchEvent(new MouseEvent('click', { bubbles: true })); } catch (e) {}
        try { if (typeof el.click === 'function') el.click(); } catch (e) {}
    }

    // 列表中找「试卷二」：文本精确匹配优先，序位第 2 兜底
    function findPaperTwoLi() {
        var lis = document.querySelectorAll('#canRunExamList > li.mui-table-view-cell');
        if (!lis || lis.length === 0) return null;
        for (var i = 0; i < lis.length; i++) {
            var name = (lis[i].innerText || lis[i].textContent || "").replace(/\s/g, "");
            if (/模拟试题二|试卷二/.test(name)) return lis[i];
        }
        return (lis.length >= 2) ? lis[1] : null;
    }

    function tick() {
        try {
            if (isDisabled()) return;
            if (inExamPage()) return; // 考试页归答题引擎

            // 状态未变化：本轮跳过（3s 后再看）
            var sig = snapshot();
            if (sig === lastSig) return;
            lastSig = sig;

            var target = pickTarget();
            if (!target) return; // 全满或数据未就绪

            var page = pageTitle();

            // ---- 考试列表页 ----
            if (page !== "") {
                if (examKeyForPage() === target && !examFull(target)) {
                    var li = findPaperTwoLi();
                    if (li) {
                        log("[" + page + "] 积分未满，点击试卷二开考");
                        fireTap(li);
                    }
                    // 没渲染出列表：等下一轮变化
                    return;
                }
                // 页面与目标不符 → 回首页
                var homeTab = document.getElementById('PersonMain');
                if (homeTab) {
                    log("[" + page + "] 与目标[" + target + "]不符，返回首页");
                    fireTap(homeTab);
                }
                return;
            }

            // ---- 壳内首页 ----
            var tabId = (target === "手机考试") ? 'mainMobileExam' : 'mainSimulate';
            var tab = document.getElementById(tabId);
            if (!tab) return;

            // 同一 Tab 连点上限保护
            if (tabId === lastTabId) {
                tabTries++;
                if (tabTries > TAB_TRIES_LIMIT) {
                    log(tabId + " 连点 " + tabTries + " 次未见变化，暂停 5 分钟");
                    lastSig = ""; // 强制下轮重新评估
                    tabTries = 0;
                    lastTabId = "";
                    busyPause();
                    return;
                }
            } else {
                lastTabId = tabId;
                tabTries = 1;
            }
            log("[" + target + "] 积分未满，进入考试列表（第 " + tabTries + " 次）");
            fireTap(tab);
        } catch (e) {
            log("轮询异常: " + e);
        }
    }

    var POLL_MS_FALLBACK = 3000;   // 未配置时的默认轮询
    var POLL_MIN = 1000, POLL_MAX = 5000; // 设置可调范围（与设置页滑块一致）
    var pausedUntil = 0;
    function busyPause() { pausedUntil = Date.now() + 300000; }

    function getPollMs() {
        var v = parseInt(window.BM_CFG && window.BM_CFG.autoStartPollMs, 10);
        if (isNaN(v)) v = POLL_MS_FALLBACK;
        return Math.max(POLL_MIN, Math.min(POLL_MAX, v));
    }

    function loop() {
        setTimeout(function() {
            if (Date.now() >= pausedUntil) tick();
            loop();
        }, getPollMs());
    }
    loop();

    document.addEventListener('bm-cfg', function() {
        setTimeout(tick, 0);
    });

    log("已注入（v3：状态检测驱动，轮询 " + getPollMs() + "ms）");
})();