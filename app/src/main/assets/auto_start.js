// 自动开考调度脚本（积分目标驱动）
// 注入主 WebView，每 5.5s 轮询（错开积分 5s 轮询）：
//   今日积分 < 24 → 底部「手机考试」→ 列表中点击【试卷二】→ 站点自动开考 → 答题引擎接管
// 数据依赖：原生层把今日积分写入 window.BM_POINTS_TOTAL（积分 LiveData 每次更新时下发）
// 安全边界：考试页绝不干预（答题引擎负责）；交卷/异常场景靠冷却时间自然恢复
(function() {
    'use strict';

    if (window.self !== window.top) return;

    // 防重复注入（整页导航后环境重置会重新注入，属预期）
    if (window.BM_AUTOSTART_INJECTED) {
        console.log("[自动开考] 已注入，跳过");
        return;
    }
    window.BM_AUTOSTART_INJECTED = true;

    // ==========================================
    // 参数（后续如需设置页可控，改为读 window.BM_CFG）
    // ==========================================
    var TARGET_POINTS = 24;      // 今日积分目标：达到即不再自动开考
    var POLL_MS = 5500;          // 轮询间隔（错开积分 5s 轮询）
    var TAB_BUSY_MS = 8000;      // 点击底部 Tab 后的冷却（等列表片段 AJAX 渲染）
    var PAPER_BUSY_MS = 180000;  // 点击试卷后的长冷却（覆盖整场考试+交卷+回跳）
    var MAX_TAB_TRIES = 3;       // Tab 连点次数上限（防 MyNotFinish 死循环）

    var busyUntil = 0;
    var tabTries = 0;

    function log(m) { console.log("[自动开考] " + m); }

    // 今日积分（原生注入；未注入时按"已达标"处理，避免误触发）
    function currentPoints() {
        var v = window.BM_POINTS_TOTAL;
        return (typeof v === "number" && !isNaN(v)) ? v : TARGET_POINTS;
    }

    // 是否处于考试页环境（答题主脚本的运行环境）
    function inExamPage() {
        try {
            return !!(window.vData && window.onlineCur && window.allShiTi);
        } catch (e) { return false; }
    }

    // 组合点击：mui 的 tap 基于委托监听（依赖事件冒泡），
    // 这里同时派发 touch 序列 + 'tap' + click，覆盖 mui tap / jQuery / 原生三种绑定
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

    // 在考试列表中找「试卷二」：文本精确匹配优先，四套题按序取第 2 个兜底
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
            // 1. 考试页：答题引擎负责，绝不干预
            if (inExamPage()) return;

            // 2. 积分达标：静默
            var pts = currentPoints();
            if (pts >= TARGET_POINTS) return;

            // 3. 冷却中：等待
            var now = Date.now();
            if (now < busyUntil) return;

            // 4. 场景 B：已在考试列表页（手机考试/模拟考试片段，SPA 装在壳内，底部 Tab 也存在）
            //    → 优先直接点试卷二
            var li = findPaperTwoLi();
            if (li) {
                log("今日积分 " + pts + " < " + TARGET_POINTS + "，点击试卷二开始考试");
                fireTap(li);
                busyUntil = now + PAPER_BUSY_MS;
                return;
            }

            // 5. 场景 A：壳内首页 → 点底部「手机考试」Tab（站点自身逻辑会 load 列表片段）
            var tab = document.getElementById('mainMobileExam');
            if (tab) {
                if (tabTries >= MAX_TAB_TRIES) {
                    log("连续点击 Tab " + tabTries + " 次未见列表，暂停 5 分钟（可能存在未完成考试）");
                    busyUntil = now + 300000;
                    tabTries = 0;
                    return;
                }
                tabTries++;
                log("今日积分 " + pts + " < " + TARGET_POINTS + "，进入手机考试（第 " + tabTries + " 次）");
                fireTap(tab);
                busyUntil = now + TAB_BUSY_MS;
                return;
            }

            // 6. 其他页面（成绩页等无壳页面）：等待回壳，不做猜测性操作
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

    // 配置热更新事件顺带触发一次（积分刚下发时不必等 5.5s）
    document.addEventListener('bm-cfg', function() {
        setTimeout(tick, 0);
    });

    log("已注入（目标 " + TARGET_POINTS + " 分，轮询 " + POLL_MS + "ms）");
})();