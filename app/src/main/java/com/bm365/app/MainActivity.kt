package com.bm365.app

import android.annotation.SuppressLint
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.Color
import android.os.Bundle
import android.view.KeyEvent
import android.view.Menu
import android.view.View
import android.webkit.CookieManager
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.PopupMenu
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.updateLayoutParams
import androidx.constraintlayout.widget.ConstraintLayout
import androidx.lifecycle.ViewModelProvider
import com.bm365.app.databinding.ActivityMainBinding

class MainActivity : AppCompatActivity() {

    private lateinit var binding: ActivityMainBinding
    private lateinit var viewModel: PointsViewModel

    /** 主 WebView — 用户交互、自动答题 */
    private lateinit var mainWebView: WebView

    /** 隐藏 WebView — 仅用于积分数据轮询，不影响主 WebView */
    private lateinit var hiddenWebView: WebView

    /** 积分桥接 */
    private lateinit var pointsBridge: PointsBridge

    /** 主 WebView 桥接（标题栏颜色） */
    private val toolbarBridge = ToolbarColorBridge()

    /** 标题栏自定义标题视图（设置入口 + 取色联动） */
    private var toolbarTitleView: TextView? = null

    /** 当前主界面标题栏颜色（网页动态取色结果），打开设置页时传给它保持一致 */
    private var currentToolbarColor: Int = Color.parseColor("#66BB6A")

    /** 积分适配器 */
    private lateinit var pointsAdapter: PointsAdapter

    /** 自动答题提示框：每次进入软件只弹一次 */
    private var autoStartPromptShown = false

    /** JS 脚本内容（从 assets 预加载） */
    private var autoAnswerScript: String = ""
    private var pointsMonitorScript: String = ""
    private var autoStartScript: String = ""

    /** 设置持久化 + 当前生效配置 */
    private lateinit var bmSettings: BmSettings
    private var currentConfig: BmConfig = BmConfig.DEFAULT

    /** 设置页启动器：关闭后立即应用新配置 */
    private lateinit var settingsLauncher: ActivityResultLauncher<Intent>

    companion object {
        // 登录后直接进入已登录主页面（Code4=0018 站点，登录态由全局 Cookie 提供）
        private const val HOME_URL = "http://61.185.41.209:8888/PersonWap/Index0018"
        private const val JS_BRIDGE_NAME = "Android"

        // 账号弹窗特殊动作
        private const val ACTION_MORE = "more"
        private const val ACTION_LOGOUT = "logout"
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // 登录门禁：未登录直接跳登录页（登录成功后才会进入本页）
        val accountManager = BmAccountManager(this)
        if (!accountManager.isLoggedIn()) {
            startActivity(Intent(this, LoginActivity::class.java))
            finish()
            return
        }

        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        // 初始化设置持久化并载入当前配置
        bmSettings = BmSettings(this)
        currentConfig = bmSettings.load()

        // 沉浸式：内容延伸到状态栏后面
        window.decorView.systemUiVisibility = (
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
            or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
        )
        // 初始状态栏颜色与标题栏一致
        window.statusBarColor = Color.parseColor("#1565C0")

        // 边缘到边缘：状态栏高度注入标题栏（顶部避让），导航栏高度注入积分卡片（底部避让）
        applyWindowInsets()

        // 入口：点击标题栏文字打开设置页（自定义 TextView 扩大热区）
        setupToolbarTitleClick()
        // 确保 Cookie 共享
        CookieManager.getInstance().apply {
            setAcceptCookie(true)
        }

        // 初始化 ViewModel
        viewModel = ViewModelProvider(this)[PointsViewModel::class.java]

        // 预加载 JS 脚本
        loadScriptsFromAssets()

        // 配置积分 RecyclerView
        setupPointsRecyclerView()

        // 观察积分数据
        observePointsData()

        // 初始化主 WebView
        initMainWebView()

        // 初始化隐藏 WebView（积分监控）
        initHiddenWebView()

        // 观察隐藏 WebView 传来的积分数据
        pointsBridge.pointsDataJson.observe(this) { json ->
            viewModel.parsePointsJson(json)
        }
        pointsBridge.errorMessage.observe(this) { error ->
            viewModel.setError(error)
        }

        // 设置页启动器：返回后把最新配置热更新到两个 WebView
        settingsLauncher = registerForActivityResult(
            ActivityResultContracts.StartActivityForResult()
        ) { applyLatestConfig() }

        // 退出登录改为电源按钮：下拉显示账号列表 + 退出登录
        binding.btnLogout.setOnClickListener { showAccountMenu(it) }

        // 加载目标网址
        mainWebView.loadUrl(HOME_URL)
        hiddenWebView.loadUrl(HOME_URL)

        // 静默检查在线更新（有新版本才提示）
        BmUpdateUi.check(this, silent = true)
    }

    /** 弹窗统一宽度：屏幕 27%，钳制 105~140dp */
    private fun popupWidth(): Int =
        ((resources.displayMetrics.widthPixels * 0.27).toInt())
            .coerceIn((105 * resources.displayMetrics.density).toInt(), (140 * resources.displayMetrics.density).toInt())

    /** 进入软件时若有考试积分未满，询问是否自动答题（拒绝则本会话停用 auto_start.js） */
    private fun maybePromptAutoStart(list: List<PointsItem>?) {
        if (autoStartPromptShown || list.isNullOrEmpty()) return
        autoStartPromptShown = true
        val unmet = mutableListOf<String>()
        list.forEach {
            if (it.name.contains("手机考试") && it.current < it.max) unmet.add("手机考试 ${it.current}/${it.max}")
            if (it.name.contains("模拟考试") && it.current < it.max) unmet.add("模拟考试 ${it.current}/${it.max}")
        }
        if (unmet.isEmpty()) return

        androidx.appcompat.app.AlertDialog.Builder(this)
            .setTitle("自动答题")
            .setMessage("当前考试积分未满：\n${unmet.joinToString("\n")}\n\n是否自动开考（自动选择试卷二作答）？")
            .setPositiveButton("自动答题") { _, _ ->
                Toast.makeText(this, "已开启自动答题，将自动选择未满项开考", Toast.LENGTH_SHORT).show()
            }
            .setNegativeButton("暂不自动") { _, _ ->
                if (::mainWebView.isInitialized) {
                    mainWebView.evaluateJavascript("window.BM_AUTOSTART_DISABLED=true;", null)
                }
            }
            .setCancelable(false)
            .show()
    }

    /** 电源按钮下拉菜单：紧凑宽度 + 账号列表（最多5个+更多折叠）+ 退出登录 */
    private fun showAccountMenu(anchor: View) {
        val am = BmAccountManager(this)
        val accounts = am.getAccounts()
        val current = am.lastAccount() ?: ""
        if (accounts.isEmpty()) return

        val shown = accounts.take(5)
        val rest = accounts.drop(5)

        // 弹窗条目：label 显示文本，account=null 表示特殊动作项
        data class Entry(val label: String, val account: String?, val action: String? = null)
        val entries = shown.map { acc ->
            val mark = if (acc == current) "✓ " else ""
            Entry(mark + am.displayName(acc), acc)
        }.toMutableList()
        if (rest.isNotEmpty()) entries.add(Entry("更多账号（${rest.size}）…", null, ACTION_MORE))
        entries.add(Entry("退出登录", null, ACTION_LOGOUT))

        val lpw = android.widget.ListPopupWindow(this)
        lpw.setAnchorView(anchor)
        // 紧凑宽度：屏幕的 27%，限制在 105~140dp
        val w = popupWidth()
        lpw.setWidth(w)
        lpw.setAdapter(android.widget.ArrayAdapter(this, android.R.layout.simple_list_item_1, entries.map { it.label }))
        lpw.setOnItemClickListener { _, _, pos, _ ->
            lpw.dismiss()
            val e = entries.getOrNull(pos) ?: return@setOnItemClickListener
            when (e.action) {
                ACTION_MORE -> showMoreAccountsMenu(anchor, rest, current)
                ACTION_LOGOUT -> confirmLogout()
                else -> if (e.account != null && e.account != current) switchAccount(e.account)
            }
        }
        lpw.show()
    }

    /** 「更多账号」二级弹窗（同样紧凑宽度） */
    private fun showMoreAccountsMenu(anchor: View, rest: List<String>, current: String) {
        val am = BmAccountManager(this)
        val lpw = android.widget.ListPopupWindow(this)
        lpw.setAnchorView(anchor)
        lpw.setWidth(popupWidth())
        lpw.setAdapter(
            android.widget.ArrayAdapter(
                this, android.R.layout.simple_list_item_1,
                rest.map { acc -> (if (acc == current) "✓ " else "") + am.displayName(acc) }
            )
        )
        lpw.setOnItemClickListener { _, _, pos, _ ->
            lpw.dismiss()
            rest.getOrNull(pos)?.let { if (it != current) switchAccount(it) }
        }
        lpw.show()
    }

    /**
     * 切换账号：清 Cookie → 网络登录（子线程）→ 成功后重载双 WebView
     * （Cookie/积分/答题引擎全部随新登录态重置，无需重启 Activity）
     */
    private fun switchAccount(account: String) {
        if (account.isBlank()) return
        val am = BmAccountManager(this)
        Toast.makeText(this, "正在切换到 ${am.displayName(account)} ...", Toast.LENGTH_SHORT).show()
        Thread {
            BmAccountManager.clearSiteCookies()
            val result = BmHttpLogin.login(account)
            runOnUiThread {
                if (result.success) {
                    am.onLoginSuccess(account, result.name)
                    Toast.makeText(this, "已切换到 ${am.displayName(account)}", Toast.LENGTH_SHORT).show()
                    // 双 WebView 重载为已登录主页
                    mainWebView.loadUrl(HOME_URL)
                    hiddenWebView.loadUrl(HOME_URL)
                } else {
                    Toast.makeText(this, "切换失败：${result.message}", Toast.LENGTH_LONG).show()
                }
            }
        }.start()
    }

    /** 退出登录确认弹窗 */
    private fun confirmLogout() {
        androidx.appcompat.app.AlertDialog.Builder(this)
            .setTitle("退出登录")
            .setMessage("退出后将清除登录状态并返回登录页，确认退出？")
            .setPositiveButton("退出") { _, _ -> performLogout() }
            .setNegativeButton("取消", null)
            .show()
    }

    /** 执行退出：清账号标记 + 清站点 Cookie，返回登录页 */
    private fun performLogout() {
        BmAccountManager(this).logout()
        BmAccountManager.clearSiteCookies()
        startActivity(Intent(this, LoginActivity::class.java))
        finish()
    }

    // ============================================================
    // 加载 assets 脚本
    // ============================================================

    private fun loadScriptsFromAssets() {
        autoAnswerScript = try {
            assets.open("auto_answer.js").bufferedReader().readText()
        } catch (e: Exception) {
            "console.log('答题脚本加载失败: ${e.message}');"
        }

        pointsMonitorScript = try {
            assets.open("points_monitor.js").bufferedReader().readText()
        } catch (e: Exception) {
            "console.log('积分脚本加载失败: ${e.message}');"
        }

        autoStartScript = try {
            assets.open("auto_start.js").bufferedReader().readText()
        } catch (e: Exception) {
            "console.log('自动开考脚本加载失败: ${e.message}');"
        }
    }

    // ============================================================
    // 积分 RecyclerView
    // ============================================================

    private fun setupPointsRecyclerView() {
        pointsAdapter = PointsAdapter()
        binding.pointsRecyclerView.apply {
            layoutManager = androidx.recyclerview.widget.GridLayoutManager(this@MainActivity, 2)
            adapter = pointsAdapter
        }
    }

    private fun observePointsData() {
        // 积分列表
        viewModel.pointsList.observe(this) { list ->
            pointsAdapter.submitList(list)
            binding.pointsRecyclerView.visibility =
                if (list.isNullOrEmpty()) View.GONE else View.VISIBLE
            // 转发考试积分明细（手机考试/模拟考试分别判断，自动开考数据源）
            pushExamPointsToMain(list)
            // 首次拿到积分明细时，若有未满项则询问是否自动答题
            maybePromptAutoStart(list)
        }

        // 总积分
        viewModel.totalPoints.observe(this) { total ->
            binding.totalPointsValue.text = total.toString()
        }

        // 状态文本
        viewModel.statusText.observe(this) { text ->
            if (!text.isNullOrBlank()) {
                binding.pointsStatusText.visibility = View.VISIBLE
                binding.pointsStatusText.text = text
            } else {
                binding.pointsStatusText.visibility = View.GONE
            }
        }
    }

    // ============================================================
    // 主 WebView（考试 + 答题）
    // ============================================================

    @SuppressLint("SetJavaScriptEnabled")
    private fun initMainWebView() {
        mainWebView = binding.webView

        // 注册标题栏颜色桥接
        mainWebView.addJavascriptInterface(toolbarBridge, "App")

        mainWebView.apply {
            with(settings) {
                javaScriptEnabled = true
                domStorageEnabled = true
                allowFileAccess = true
                databaseEnabled = true
                useWideViewPort = true
                loadWithOverviewMode = true
                setSupportZoom(true)
                builtInZoomControls = true
                displayZoomControls = false
                mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
                // 允许 HTTP 明文
                blockNetworkLoads = false
            }

            webViewClient = MainWebViewClient()
            webChromeClient = object : WebChromeClient() {
                override fun onProgressChanged(view: WebView?, newProgress: Int) {
                    if (newProgress == 100) {
                        binding.progressBar.visibility = View.GONE
                    } else {
                        binding.progressBar.visibility = View.VISIBLE
                        binding.progressBar.progress = newProgress
                    }
                }
            }
        }
    }

    private inner class MainWebViewClient : WebViewClient() {
        override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
            super.onPageStarted(view, url, favicon)
            binding.progressBar.visibility = View.VISIBLE
            binding.progressBar.progress = 0
        }

        override fun onPageFinished(view: WebView?, url: String?) {
            super.onPageFinished(view, url)
            // 先下发配置（首次注入，不派发事件），再注入答题脚本 + 自动开考调度脚本
            pushConfigTo(mainWebView, fireEvent = false)
            injectAutoAnswerScript()
            injectAutoStartScript()
            // 提取页面主题色
            extractPageColor()
        }

        override fun shouldOverrideUrlLoading(
            view: WebView?,
            request: WebResourceRequest?
        ): Boolean {
            val url = request?.url ?: return false
            // 外部协议（tel:/mailto:/intent: 等）交给系统处理，其余全部在 WebView 内打开
            return if (url.scheme == "http" || url.scheme == "https") false else true
        }
    }

    private fun injectAutoAnswerScript() {
        if (autoAnswerScript.isNotBlank()) {
            mainWebView.evaluateJavascript(autoAnswerScript, null)
        }
    }

    /** 把考试积分明细转发给主 WebView：{"手机考试":{"cur":24,"max":24},...}（auto_start.js 数据源） */
    private fun pushExamPointsToMain(list: List<PointsItem>?) {
        if (!::mainWebView.isInitialized) return
        val json = list.orEmpty()
            .filter { it.name.contains("手机考试") || it.name.contains("模拟考试") }
            .joinToString(",", "{", "}") {
                "\"${it.name}\":{\"cur\":${it.current},\"max\":${it.max}}"
            }
        mainWebView.evaluateJavascript("window.BM_EXAM_POINTS=$json;", null)
    }

    /** 注入自动开考调度脚本（积分 <24 → 自动进手机考试 → 点试卷二） */
    private fun injectAutoStartScript() {
        if (autoStartScript.isNotBlank()) {
            mainWebView.evaluateJavascript(autoStartScript, null)
        }
    }

    /**
     * 提取页面首个非黑白 div 的背景色，回传设置标题栏
     */
    private fun extractPageColor() {
        val js = """
            (function(){
                var divs = document.querySelectorAll('div');
                for(var i = 0; i < divs.length; i++){
                    var bg = window.getComputedStyle(divs[i]).backgroundColor;
                    if(bg && bg !== 'rgba(0, 0, 0, 0)' && bg !== 'rgb(0, 0, 0)' && bg !== 'rgb(255, 255, 255)' && bg !== 'transparent'){
                        App.setToolbarColor(bg);
                        return;
                    }
                }
            })();
        """.trimIndent()
        mainWebView.evaluateJavascript(js, null)
    }

    /**
     * 标题栏颜色桥接
     */
    inner class ToolbarColorBridge {
        @android.webkit.JavascriptInterface
        fun setToolbarColor(colorStr: String) {
            runOnUiThread {
                try {
                    val color = parseCssColor(colorStr)
                    // 记录当前色值，供设置页复用（标题栏/状态栏同色）
                    currentToolbarColor = color

                    // 标题栏背景
                    binding.toolbar.setBackgroundColor(color)
                    // 状态栏背景与标题栏完全一致
                    window.statusBarColor = color

                    // 标题文字颜色：白色/极浅底用黑字，其余（含绿色）用白字
                    val textColor = if (isVeryLightColor(color)) Color.BLACK else Color.WHITE
                    binding.toolbar.setTitleTextColor(textColor)
                    // 自定义标题视图同步变色（设置入口 TextView）
                    toolbarTitleView?.setTextColor(textColor)

                    // 状态栏图标：白底用深色图标，其余用浅色
                    window.decorView.systemUiVisibility = (
                        View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                        or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                        or if (isVeryLightColor(color)) View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR else 0
                    )
                } catch (e: Exception) {
                    // 解析失败，保持默认
                }
            }
        }
    }

    /** 判断颜色是否接近白色（亮度 > 0.85），用于决定文字/图标深浅 */
    private fun isVeryLightColor(color: Int): Boolean {
        val r = Color.red(color)
        val g = Color.green(color)
        val b = Color.blue(color)
        val luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255.0
        return luminance > 0.85
    }

    private fun parseCssColor(css: String): Int {
        val trimmed = css.trim()
        return when {
            trimmed.startsWith("rgba(") || trimmed.startsWith("rgb(") -> {
                val parts = trimmed.substringAfter('(').substringBefore(')')
                    .split(',').map { it.trim().toIntOrNull() ?: 0 }
                Color.rgb(parts.getOrElse(0) { 0 }, parts.getOrElse(1) { 0 }, parts.getOrElse(2) { 0 })
            }
            trimmed.startsWith("#") -> Color.parseColor(trimmed)
            else -> throw IllegalArgumentException("无法解析颜色: $trimmed")
        }
    }

    // ============================================================
    // 隐藏 WebView（积分监控，独立运行，不影响主 WebView）
    // ============================================================

    @SuppressLint("SetJavaScriptEnabled")
    private fun initHiddenWebView() {
        hiddenWebView = WebView(this).apply {
            // 1×1 像素，不可见但可运行 JS
            layoutParams = ConstraintLayout.LayoutParams(1, 1).apply {
                startToStart = ConstraintLayout.LayoutParams.PARENT_ID
                topToTop = ConstraintLayout.LayoutParams.PARENT_ID
            }
            visibility = View.VISIBLE
            isFocusable = false
            isFocusableInTouchMode = false
            isClickable = false

            with(settings) {
                javaScriptEnabled = true
                domStorageEnabled = true
                allowFileAccess = true
                // 不加载图片，减少资源消耗
                loadsImagesAutomatically = false
                blockNetworkImage = true
                // 禁用不必要功能
                javaScriptCanOpenWindowsAutomatically = false
                setSupportZoom(false)
                mediaPlaybackRequiresUserGesture = true
                mixedContentMode = android.webkit.WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
            }

            // 添加 JS 桥接
            pointsBridge = PointsBridge()
            addJavascriptInterface(pointsBridge, JS_BRIDGE_NAME)

            webViewClient = HiddenWebViewClient()
        }

        // 添加到根布局
        binding.root.addView(hiddenWebView)
    }

    private inner class HiddenWebViewClient : WebViewClient() {
        override fun onPageFinished(view: WebView?, url: String?) {
            super.onPageFinished(view, url)
            // 页面加载完成：先下发配置，再注入积分监控脚本
            pushConfigTo(hiddenWebView, fireEvent = false)
            injectPointsMonitorScript()
        }

        override fun shouldOverrideUrlLoading(
            view: WebView?,
            request: WebResourceRequest?
        ): Boolean {
            // 阻止隐藏 WebView 跳转，保持在同一页面
            val url = request?.url?.toString() ?: ""
            return if (url.contains("61.185.41.209")) {
                false // 允许同站导航
            } else {
                true // 阻止外站跳转
            }
        }
    }

    private fun injectPointsMonitorScript() {
        if (pointsMonitorScript.isNotBlank()) {
            hiddenWebView.evaluateJavascript(pointsMonitorScript, null)
        }
    }

    // ============================================================
    // 设置入口：标题绝对居中（约束父容器两端，不受电源按钮影响）
    // ============================================================

    private fun setupToolbarTitleClick() {
        binding.toolbarTitle.setOnClickListener { openSettings() }
        // 标题视图与取色联动共用
        toolbarTitleView = binding.toolbarTitle
    }

    private fun openSettings() {
        settingsLauncher.launch(
            Intent(this, SettingsActivity::class.java)
                .putExtra(SettingsActivity.EXTRA_TOOLBAR_COLOR, currentToolbarColor)
        )
    }

    /** 边缘到边缘 insets：标题栏内容整体下沉到状态栏以下可视区，积分卡片避让导航栏 */
    private fun applyWindowInsets() {
        val density = resources.displayMetrics.density
        ViewCompat.setOnApplyWindowInsetsListener(binding.root) { _, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars())
            // 顶部：Toolbar 总高 = 状态栏 + 内容区；内容锚点下沉到状态栏以下（标题/按钮都锚定它）
            binding.toolbar.updateLayoutParams<android.view.ViewGroup.LayoutParams> {
                height = bars.top + (42 * density).toInt()
            }
            binding.toolbarContentAnchor.updateLayoutParams<androidx.constraintlayout.widget.ConstraintLayout.LayoutParams> {
                topMargin = bars.top
            }
            // 底部：积分卡片追加导航栏高度的外边距
            binding.pointsPanel.updateLayoutParams<android.view.ViewGroup.MarginLayoutParams> {
                bottomMargin = bars.bottom + (6 * density).toInt()
            }
            insets
        }
        ViewCompat.requestApplyInsets(binding.root)
    }

    // ============================================================
    // 配置下发：window.BM_CFG + bm-cfg 事件 + BM_SET_POLL 兜底
    // ============================================================

    /** 设置页返回后调用：重读持久化配置并热更新到两个 WebView（不重载页面） */
    fun applyLatestConfig() {
        currentConfig = bmSettings.load()
        pushConfigTo(mainWebView, fireEvent = true)
        pushConfigTo(hiddenWebView, fireEvent = true)
    }

    /** 向指定 WebView 下发配置对象 window.BM_CFG */
    private fun pushConfigTo(target: WebView, fireEvent: Boolean) {
        val js = buildCfgInjectionJs(currentConfig, fireEvent)
        target.evaluateJavascript(js, null)
    }

    /**
     * 构造配置注入 JS：
     * 1) 全量替换 window.BM_CFG
     * 2) fireEvent=true 时派发 'bm-cfg' 事件（脚本自重排链立即按新配置重排）
     * 3) 兜底桥：window.BM_SET_POLL(ms)（仅积分 WebView 存在）
     */
    private fun buildCfgInjectionJs(cfg: BmConfig, fireEvent: Boolean): String {
        val eventPart = if (fireEvent) "document.dispatchEvent(new Event('bm-cfg'));" else ""
        return """
            (function(){
                try {
                    window.BM_CFG = ${cfg.toJson()};
                    $eventPart
                    if (typeof window.BM_SET_POLL === 'function') {
                        window.BM_SET_POLL(${cfg.pointsPollMs});
                    }
                } catch(e) { console.log('[BM] 配置下发失败:', e); }
            })();
        """.trimIndent()
    }

    // ============================================================
    // 返回键处理：主 WebView 优先回退
    // ============================================================

    override fun onKeyDown(keyCode: Int, event: KeyEvent?): Boolean {
        if (keyCode == KeyEvent.KEYCODE_BACK && ::mainWebView.isInitialized && mainWebView.canGoBack()) {
            mainWebView.goBack()
            return true
        }
        return super.onKeyDown(keyCode, event)
    }

    // ============================================================
    // 生命周期
    // ============================================================

    override fun onPause() {
        super.onPause()
        if (::mainWebView.isInitialized) mainWebView.onPause()
        if (::hiddenWebView.isInitialized) hiddenWebView.onPause()
    }

    override fun onResume() {
        super.onResume()
        if (::mainWebView.isInitialized) mainWebView.onResume()
        if (::hiddenWebView.isInitialized) hiddenWebView.onResume()
    }

    override fun onDestroy() {
        // 清理隐藏 WebView
        if (::hiddenWebView.isInitialized) {
            hiddenWebView.removeJavascriptInterface(JS_BRIDGE_NAME)
            hiddenWebView.destroy()
        }
        // 清理主 WebView
        if (::mainWebView.isInitialized) {
            mainWebView.removeJavascriptInterface("App")
            mainWebView.destroy()
        }
        super.onDestroy()
    }
}
