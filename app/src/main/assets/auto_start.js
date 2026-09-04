// 自动开考调度脚本 v2（积分明细分别判断）
// 注入主 WebView，每 5.5s 轮询（错开积分 5s 轮询）：
//   「手机考试」积分未满 → 自动进手机考试列表 → 点【试卷二】→ 答题引擎接管
//   「模拟考试」积分未满 → 自动进模拟考试列表 → 点【试卷二】→ 答题引擎接管
//   两项都满 → 静默
// 数据依赖：原生层把积分明细写入 window.BM_EXAM_POINTS = {"手机考试":{"cur":N,"max":M}, "模拟考试":{...}}
// 安全边界：考试页绝不干预（答题引擎负责）；冷却时间防重复；数据未就绪时不动
(function() {
    'use strict';

    if (window.self !== window.top) return;

    if (window.BM_AUTOSTART_INJECTED) {
        console.log("[自动开考] 已注入，跳过");
        return;
    }
    window.BM_AUTOSTART_INJECTED = true;

    // ==========================================
    // 参数
    // ==========================================
    var POLL_MS = 5500;          // 轮询间隔（错开积分 5s 轮询）
    var TAB_BUSY_MS = 8000;      // 点 Tab 后冷却（等列表片段渲染）
    var PAPER_BUSY_MS = 180000;  // 点试卷后长冷却（覆盖整场考试+交卷+回跳）
    var MAX_TAB_TRIES = 3;       // Tab 连点上限（防未完成考试循环）

    var busyUntil = 0;
    var tabTries = 0;

    function log(m) { console.log("[自动开考] " + m); }

    // 页面标题 → 考试类别（列表页片段有 mui-title；壳内首页无）
    function pageTitle() {
        var t = document.querySelector('.mui-bar-nav .mui-title');
        return t ? (t.textContent || "").trim() : "";
    }

    // 列表页标题对应的积分明细键名
    function examKeyForPage() {
        var t = pageTitle();
        if (t.indexOf("手机考试") !== -1) return "手机考试";
        if (t.indexOf("模拟考试") !== -1) return "模拟考试";
        return "";
    }

    // 当前页面的考试类别是否积分已满（数据缺失时视为已满=不操作）
    function isCurrentPageExamFull() {
        var key = examKeyForPage();
        if (!key) return true;
        try {
            var d = window.BM_EXAM_POINTS || {};
            var e = d[key];
            if (!e || typeof e.cur !== "number" || typeof e.max !== "number") return true;
            return e.cur >= e.max;
        } catch (err) { return true; }
    }

    // 选出当前需要补分的考试类别：手机考试优先，其次模拟考试；null=全部已满/数据未就绪
    function pickTarget() {
        try {
            var d = window.BM_EXAM_POINTS;
            if (!d || typeof d !== "object") return null;
            var mobile = d["手机考试"], sim = d["模拟考试"];
            // 明细存在且 cur/max 为数字才认为数据就绪；缺失视为已满（不误触发）
            var mobileFull = !(mobile && typeof mobile.cur === "number") || mobile.cur >= (mobile.max || 0);
            var simFull = !(sim && typeof sim.cur === "number") || sim.cur >= (sim.max || 0);
            if (!mobileFull) return "手机考试";
            if (!simFull) return "模拟考试";
            return null;
        } catch (e) { return null; }
    }

    // 是否处于考试页环境（答题引擎的领域）
    function inExamPage() {
        try {
            return !!(window.vData && window.onlineCur && window.allShiTi);
        } catch (e) { return false; }
    }

    // 组合点击：mui tap 委托 + jQuery + 原生全覆盖
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
            // 1. 考试页：答题引擎负责
            if (inExamPage()) return;

            var now = Date.now();
            if (now < busyUntil) return;

            // 2. 数据未就绪：静默等积分轮询刷出明细
            var target = pickTarget();
            if (!target) return;

            var page = pageTitle();

            // 3. 处于某个考试列表页
            if (page !== "") {
                if (examKeyForPage() === target && !isCurrentPageExamFull()) {
                    // 当前列表页正是目标考试且未满 → 点试卷二
                    var li = findPaperTwoLi();
                    if (li) {
                        log("[" + page + "] 积分未满，点击试卷二开考");
                        fireTap(li);
                        busyUntil = now + PAPER_BUSY_MS;
                        return;
                    }
                    // 列表还没渲染出来：等下一轮
                    return;
                }
                // 当前列表页不是目标（比如手机考试已满该去模拟考试）→ 点「学」回首页
                var homeTab = document.getElementById('PersonMain');
                if (homeTab) {
                    log("[" + page + "] 与目标[" + target + "]不符，返回首页切换目标");
                    fireTap(homeTab);
                    busyUntil = now + TAB_BUSY_MS;
                }
                return;
            }

            // 4. 壳内首页 → 点底部 Tab 进入目标考试列表
            var tabId = (target === "手机考试") ? 'mainMobileExam' : 'mainSimulate';
            var tab = document.getElementById(tabId);
            if (tab) {
                if (tabTries >= MAX_TAB_TRIES) {
                    log("连续点击 Tab " + tabTries + " 次未见列表，暂停 5 分钟（可能存在未完成考试）");
                    busyUntil = now + 300000;
                    tabTries = 0;
                    return;
                }
                tabTries++;
                log("[" + target + "] 积分未满，进入考试列表（第 " + tabTries + " 次）");
                fireTap(tab);
                busyUntil = now + TAB_BUSY_MS;
                return;
            }

            // 5. 其他页面（成绩页等无壳页面）：等待回壳
        } catch (e) {
            log("轮询异常: " + e);
        }
    }

    function loop() {
        setTimeout(function() {
            tick();
            loop();
        }, POLL_MS);
    }
    loop();

    // bm-cfg 热更新事件顺带触发一次
    document.addEventListener('bm-cfg', function() {
        setTimeout(tick, 0);
    });

    log("已注入（v2：手机考试/模拟考试分别判断，轮询 " + POLL_MS + "ms）");
})();