// 网页全自动答题助手_极简考试版（配置化改造版）
// 注入主 WebView，通过 setTimeout 自重排链实现可调轮询，
// 全部参数实时读取 window.BM_CFG（由原生层下发，热更新立即生效）
(function() {
    'use strict';

    if (window.self !== window.top) return;

    // 防重复注入：脚本由原生在 onPageFinished 与配置热更新时多次注入
    if (window.BM_AUTO_INJECTED) {
        console.log("[答题引擎] 已注入，跳过重复初始化（配置由 BM_APPLY_CFG 热更新）");
        return;
    }
    window.BM_AUTO_INJECTED = true;

    // ==========================================
    // 配置读取：原生注入 window.BM_CFG（全量替换），并派发 'bm-cfg' 事件
    // ==========================================
    if (!window.BM_CFG) {
        window.BM_CFG = { examPollMs: 200, answerDelayMs: 50, lockMs: 80, pointsPollMs: 5000, totalQuestions: 40, autoSubmit: true };
    }

    function cfg() { return window.BM_CFG || {}; }
    function getPollMs() {
        var v = parseInt(cfg().examPollMs, 10);
        return isNaN(v) ? 200 : v;
    }
    function getAnswerDelayMs() {
        var v = parseInt(cfg().answerDelayMs, 10);
        return isNaN(v) ? 50 : v;
    }
    function getLockMs() {
        var v = parseInt(cfg().lockMs, 10);
        return isNaN(v) ? 80 : v;
    }
    function getTotalQuestions() {
        var v = parseInt(cfg().totalQuestions, 10);
        if (isNaN(v) || v < 1) return 40;
        return v;
    }
    function isAutoSubmit() { return cfg().autoSubmit !== false; }

    // 供原生调试/兜底调用：window.BM_APPLY_CFG('{"totalQuestions":50,...}')
    window.BM_APPLY_CFG = function(json) {
        try {
            var obj = JSON.parse(json);
            window.BM_CFG = obj;
            document.dispatchEvent(new Event('bm-cfg'));
        } catch (e) { console.log("[答题引擎] 配置应用失败:", e); }
    };

    // ==========================================
    // 基础物理点击
    // ==========================================
    function fireAbsoluteCluster(element) {
        if (!element) return;
        let current = element;
        for (let i = 0; i < 3; i++) {
            if (!current || current.tagName === 'BODY' || current.tagName === 'HTML') break;
            try {
                if (current.tagName === 'INPUT') current.checked = true;
                current.dispatchEvent(new TouchEvent('touchstart', { bubbles: true, cancelable: true }));
                current.dispatchEvent(new TouchEvent('touchend', { bubbles: true, cancelable: true }));
                current.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
                current.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }));
                current.dispatchEvent(new Event('click', { bubbles: true }));
                current.dispatchEvent(new Event('change', { bubbles: true }));
                if (typeof current.click === 'function') current.click();
            } catch (e) {}
            current = current.parentElement;
        }
    }

    function findElementByTextFuzzy(keyword) {
        let elements = Array.from(document.querySelectorAll('button, a, div, span, p, li, input'));
        let matched = [];
        elements.forEach(el => {
            let rect = el.getBoundingClientRect();
            if (rect.width === 0 || rect.height === 0) return;
            let txt = (el.innerText || el.textContent || el.value || "").replace(/\s/g, "");
            let target = keyword.replace(/\s/g, "");
            if (txt.includes(target)) matched.push(el);
        });
        if (matched.length > 0) {
            matched.sort((a, b) => {
                let aLen = (a.innerText || a.textContent || "").trim().length;
                let bLen = (b.innerText || b.textContent || "").trim().length;
                return aLen - bLen;
            });
            return matched[0];
        }
        return null;
    }


    // ==========================================
    // 考试引擎
    // ==========================================
    let actionLock = false;
    let lastProcessedNum = -1;
    let hasPowerClicked = false;

    // 解锁器：无锁状态下周期性重置 lastProcessedNum，
    // 处理"下一题点击失败"等异常场景的重试（间隔 1.2s 固定）
    setInterval(() => {
        if (!actionLock) lastProcessedNum = -1;
    }, 1200);

    function runExamEngine() {
        // ========== 1. 交卷按钮（结束考试/确认交卷）==========
        // 仅在自动交卷开启时执行；关闭后答满题数也停留在答题页
        if (isAutoSubmit()) {
            let allElements = document.querySelectorAll('*');
            for (let i = 0; i < allElements.length; i++) {
                let el = allElements[i];
                if (el.children.length === 0 && el.innerText) {
                    let pureText = el.innerText.replace(/\s/g, "");
                    if (pureText === "结束考试" || pureText === "确认交卷") {
                        let rect = el.getBoundingClientRect();
                        if (rect.width > 0 && rect.height > 0) {
                            console.log(`【交卷】发现[${pureText}]，点击交卷`);
                            fireAbsoluteCluster(el);
                            return;
                        }
                    }
                }
            }

            // 备用：模糊匹配
            let endBtn = findElementByTextFuzzy('结束考试');
            if (endBtn) {
                console.log("【交卷】点击结束考试");
                fireAbsoluteCluster(endBtn);
                return;
            }
            let confirmBtn = findElementByTextFuzzy('确认交卷');
            if (confirmBtn) {
                console.log("【交卷】点击确认交卷");
                fireAbsoluteCluster(confirmBtn);
                return;
            }
        }

        // ========== 2. 答题核心 ==========
        const W = typeof unsafeWindow !== 'undefined' ? unsafeWindow : window;

        if (!(W.vData && W.onlineCur && W.allShiTi)) return;
        if (actionLock) return;

        let TOTAL_QUESTIONS = getTotalQuestions(); // 交卷阈值动态化：每次循环实时读取
        let currentNum = parseInt(W.onlineCur, 10);
        if (currentNum > TOTAL_QUESTIONS) return;

        // 最后一题交卷逻辑
        if (currentNum === lastProcessedNum) {
            if (currentNum === TOTAL_QUESTIONS) {
                let hasAnswered = (W.userAns || W.answer || document.querySelector('input[type="radio"]:checked'));
                if (hasAnswered && !hasPowerClicked) {
                    if (!isAutoSubmit()) {
                        console.log("【交卷】自动交卷已关闭，停留在最后一题");
                        return;
                    }
                    console.log(`【交卷】第${currentNum}题已答，执行交卷`);
                    hasPowerClicked = true;

                    let powerBtn = document.querySelector('.fa-power-off, [class*="power"], .glyphicon-off, [style*="red"]');
                    if (!powerBtn) {
                        let headers = Array.from(document.querySelectorAll('div, span, p')).filter(el => el.innerText && (el.innerText.includes("题") || el.innerText.includes("分钟")));
                        if (headers.length > 0) powerBtn = headers[0].parentElement.querySelector('.red, i, [style*="color: red"], [style*="color:red"]');
                    }

                    if (powerBtn) {
                        fireAbsoluteCluster(powerBtn);
                    } else {
                        if (typeof W.saveAndExit === 'function') W.saveAndExit();
                        else if (typeof W.tijiao === 'function') W.tijiao();
                    }
                }
            }
            return;
        }

        if (currentNum < TOTAL_QUESTIONS && hasPowerClicked) {
            hasPowerClicked = false;
        }

        // 答题
        try {
            let ansSign = "";

            // 方法1：通过题目文本匹配
            let titleEl = document.querySelector('div[class*="title"], p[class*="title"], .main, h4');
            let titleText = titleEl ? titleEl.innerText.replace(/^\d+[\s.、]*/, "").substring(0, 7).trim() : "";
            if (titleText) {
                let matched = W.allShiTi.find(item => item && item.replace(/\s/g, "").includes(titleText));
                if (matched) {
                    let parts = matched.split(',');
                    ansSign = parts[13] ? parts[13].replace(/["']/g, "").trim().toUpperCase() : "";
                }
            }

            // 方法2：通过题号直接取
            if (!ansSign) {
                let raw = W.allShiTi[currentNum - 1];
                if (raw) {
                    let parts = raw.split(',');
                    let potential = parts[13] ? parts[13].replace(/["']/g, "").trim().toUpperCase() : "";
                    if (["A","B","C","D","Y","N"].includes(potential)) ansSign = potential;
                }
            }

            if (!ansSign) return;

            actionLock = true;
            lastProcessedNum = currentNum;
            console.log(`【答题】第${currentNum}题 答案: ${ansSign}`);

            let targetIdx = (ansSign === "Y" || ansSign === "N") ? (ansSign === "Y" ? 0 : 1) : ["A","B","C","D"].indexOf(ansSign);
            let targetElement = null;

            // 查找radio按钮
            let radios = Array.from(document.querySelectorAll('input[type="radio"]')).filter(r => r.getBoundingClientRect().width > 0);
            if (radios.length > 0) {
                let itemsCount = (ansSign === "Y" || ansSign === "N") ? 2 : 4;
                let currentRadios = radios.slice(-itemsCount);
                if (targetIdx !== -1 && currentRadios[targetIdx]) targetElement = currentRadios[targetIdx];
            }

            // 备用：查找文本块
            if (!targetElement && targetIdx !== -1) {
                let blocks = Array.from(document.querySelectorAll('div, li, label, p')).filter(el => {
                    let txt = el.innerText ? el.innerText.trim() : "";
                    let isOpt = txt.startsWith('A') || txt.startsWith('B') || txt.startsWith('C') || txt.startsWith('D') || txt.startsWith('对') || txt.startsWith('错') || txt === '对' || txt === '错';
                    return isOpt && el.getBoundingClientRect().height >= 15;
                });
                blocks.sort((a, b) => a.getBoundingClientRect().top - b.getBoundingClientRect().top);
                let itemsCount = (ansSign === "Y" || ansSign === "N") ? 2 : 4;
                if (blocks.length >= itemsCount) {
                    let currentBlocks = blocks.slice(-itemsCount);
                    if (currentBlocks[targetIdx]) targetElement = currentBlocks[currentBlocks.length - itemsCount + targetIdx];
                } else if (blocks[targetIdx]) {
                    targetElement = blocks[targetIdx];
                }
            }

            if (targetElement) {
                // 记录答案
                try {
                    let ansStr = (ansSign === "Y") ? "对" : ((ansSign === "N") ? "错" : ansSign);
                    if (W.userAns) W.userAns = ansStr;
                    if (W.answer) W.answer = ansStr;
                } catch(e) {}

                fireAbsoluteCluster(targetElement);

                // 点击下一题（答题后等待延时实时读取 BM_CFG.answerDelayMs）
                setTimeout(() => {
                    if (typeof W.ToNext === 'function') {
                        W.ToNext();
                        setTimeout(() => { actionLock = false; }, getLockMs());
                    } else {
                        let nextBtn = findElementByTextFuzzy('下一题') || findElementByTextFuzzy('下一页');
                        if (nextBtn) fireAbsoluteCluster(nextBtn);
                        setTimeout(() => { actionLock = false; }, getLockMs());
                    }
                }, getAnswerDelayMs());
            } else {
                setTimeout(() => { actionLock = false; }, getLockMs());
            }
        } catch (e) {
            actionLock = false;
        }
    }

    // ==========================================
    // 启动：setTimeout 自重排链
    // 每次执行完毕按最新 BM_CFG.examPollMs 重排下一次，
    // 设置页修改后下一轮自动按新间隔运行，无需刷新页面
    // ==========================================
    let stopped = false;
    function scheduleLoop() {
        if (stopped) return;
        setTimeout(() => {
            try { runExamEngine(); } catch (e) { console.log("[答题引擎] 异常:", e); }
            scheduleLoop();
        }, getPollMs());
    }
    scheduleLoop();

    // 配置热更新事件：立即执行一次，让新配置（如总题数/交卷开关）马上生效
    document.addEventListener('bm-cfg', function() {
        console.log("[答题引擎] 配置已热更新:", JSON.stringify(cfg()));
        setTimeout(() => { try { runExamEngine(); } catch (e) {} }, 0);
    });

    console.log("[答题引擎] 已注入（配置化版）");
})();