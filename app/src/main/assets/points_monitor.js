// 积分监控脚本（配置化改造版 — 无UI，纯数据回传原生层）
// 在隐藏 WebView 中运行，不影响主 WebView 的考试页面。
// 轮询间隔通过 window.BM_CFG.pointsPollMs 配置，支持 BM_SET_POLL(ms) 热更新。
(function () {
    "use strict";

    // 防重复注入
    if (window.BM_POINTS_INJECTED) {
        console.log("[积分监控] 已注入，跳过重复初始化（配置由 BM_SET_POLL 热更新）");
        return;
    }
    window.BM_POINTS_INJECTED = true;

    // ==========================================
    // 配置：原生注入 window.BM_CFG（全量替换），并派发 'bm-cfg' 事件
    // ==========================================
    if (!window.BM_CFG) {
        window.BM_CFG = { examPollMs: 200, answerDelayMs: 50, lockMs: 80, pointsPollMs: 5000, totalQuestions: 40, autoSubmit: true };
    }

    var MIN_POLL = 500; // 下限保护，防止过快轮询打爆服务器
    var isRunning = false;
    var pollTimer = null;

    /** 读取当前轮询间隔（ms），异常时回落 5000。兼容旧字段 _bmPollMs */
    function getPollMs() {
        var v = parseInt(window.BM_CFG && window.BM_CFG.pointsPollMs, 10);
        if (isNaN(v) || v < MIN_POLL) {
            v = parseInt(window._bmPollMs, 10);
        }
        if (isNaN(v) || v < MIN_POLL) v = 5000;
        return v;
    }

    /** 热更新接口：原生直接调用 window.BM_SET_POLL(ms) */
    window.BM_SET_POLL = function (ms) {
        ms = parseInt(ms, 10);
        if (isNaN(ms) || ms < MIN_POLL) ms = 5000;
        try {
            window.BM_CFG = window.BM_CFG || {};
            window.BM_CFG.pointsPollMs = ms;
        } catch (e) {}
        console.log("[积分监控] 轮询间隔已热更新为 " + ms + "ms");
        // 立即重排下一次轮询，新间隔马上生效
        if (isRunning) {
            stopPolling();
            startPolling();
        }
        return ms;
    };

    /** 配置全量替换事件（与答题脚本同通道）：重读 pointsPollMs 并重排 */
    document.addEventListener('bm-cfg', function () {
        if (isRunning) {
            stopPolling();
            startPolling();
        }
    });

    /**
     * 等待页面环境就绪（M_PersonId）
     */
    function waitForReady(callback, maxRetries) {
        maxRetries = maxRetries || 30; // 最多等30秒
        var retries = 0;

        function check() {
            retries++;
            if (typeof M_PersonId !== "undefined" && M_PersonId) {
                callback();
            } else if (retries < maxRetries) {
                setTimeout(check, 1000);
            } else {
                // 超时，报告未登录
                try {
                    Android.onError("等待登录超时，请先在主页面登录");
                } catch (e) {
                    console.log("[积分监控] 未登录，等待中...");
                }
                // 继续等待
                if (retries < 120) {
                    setTimeout(check, 3000);
                }
            }
        }

        check();
    }

    /**
     * 获取积分数据
     */
    function fetchPoints() {
        if (typeof M_PersonId === "undefined" || !M_PersonId) {
            try { Android.onError("未检测到登录信息"); } catch (e) {}
            return;
        }

        // 优先使用 jQuery.ajax，备选使用 fetch
        function doRequest() {
            if (typeof $ !== "undefined" && typeof $.ajax === "function") {
                $.ajax({
                    url: "/AccumulateManger/S_Accumulate/GetPersonTodayAccumulateOne",
                    type: "GET",
                    data: { pid: Esdt(M_PersonId) },
                    success: function (data) {
                        if (data.success && data.data) {
                            try {
                                Android.onPointsData(JSON.stringify(data.data));
                            } catch (e) {
                                console.log("[积分监控] 回传失败:", e);
                            }
                        } else {
                            try { Android.onError("获取积分数据失败"); } catch (e) {}
                        }
                    },
                    error: function (xhr, status, error) {
                        try { Android.onError("网络请求失败: " + error); } catch (e) {}
                    }
                });
            } else if (typeof fetch === "function") {
                // 备选：使用 fetch（需要处理 Esdt 加密）
                var encryptedPid = typeof Esdt === "function" ? Esdt(M_PersonId) : M_PersonId;
                fetch("/AccumulateManger/S_Accumulate/GetPersonTodayAccumulateOne?pid=" + encodeURIComponent(encryptedPid), {
                    method: "GET",
                    credentials: "same-origin"
                })
                .then(function (resp) { return resp.json(); })
                .then(function (data) {
                    if (data.success && data.data) {
                        try {
                            Android.onPointsData(JSON.stringify(data.data));
                        } catch (e) {}
                    }
                })
                .catch(function (err) {
                    try { Android.onError("请求失败: " + err.message); } catch (e) {}
                });
            }
        }

        doRequest();
    }

    /**
     * 开始轮询（setTimeout 自重排链：每次执行后按最新 BM_CFG.pointsPollMs 重排）
     */
    function startPolling() {
        if (isRunning) return;
        isRunning = true;

        var loop = function () {
            pollTimer = setTimeout(function () {
                try { fetchPoints(); } catch (e) { console.log("[积分监控] 异常:", e); }
                if (isRunning) loop();
            }, getPollMs());
        };

        // 首次立即获取
        fetchPoints();
        loop();

        console.log("[积分监控] 已启动，间隔 " + getPollMs() + "ms");
    }

    /**
     * 停止轮询
     */
    function stopPolling() {
        isRunning = false;
        if (pollTimer) {
            clearTimeout(pollTimer);
            pollTimer = null;
        }
    }

    // 等待页面就绪后启动
    waitForReady(startPolling);

    console.log("[积分监控] 脚本已注入（无UI模式，可配置轮询）");
})();