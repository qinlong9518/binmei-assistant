package com.bm365.app

import android.content.res.Resources
import android.graphics.Color
import android.os.Bundle
import android.util.TypedValue
import android.view.View
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.updatePadding
import com.bm365.app.databinding.ActivitySettingsBinding

/**
 * 设置页：滑块 + 开关，实时预览数值。
 * 保存后通过 setResult 通知 MainActivity 立即向两个 WebView 下发新配置。
 * 主题色与主界面标题栏一致（绿色沉浸式，由 Intent 传入动态色值）。
 */
class SettingsActivity : AppCompatActivity() {

    private lateinit var binding: ActivitySettingsBinding
    private lateinit var settings: BmSettings

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // inflate 防护：异常环境下降级为提示退出，避免整应用闪退循环
        binding = try {
            ActivitySettingsBinding.inflate(layoutInflater)
        } catch (t: Throwable) {
            reportFatal("设置页加载失败", t)
            finish()
            return
        }
        setContentView(binding.root)

        try {
            // 应用主界面标题栏同款颜色（默认站点绿），沉浸至状态栏
            applyImmersiveTheme(
                intent.getIntExtra(EXTRA_TOOLBAR_COLOR, Color.parseColor("#66BB6A"))
            )

            settings = BmSettings(this)

            // 顶部栏返回
            binding.settingsToolbar.setNavigationOnClickListener { finish() }

            // 载入当前配置到 UI
            bindFromConfig(settings.load())

            setupListeners()
            applyBottomInset()

            // 版本信息 + 检查更新
            binding.versionText.text = "当前版本 v${BmUpdate.currentVersionName(this)}"
            binding.btnCheckUpdate.setOnClickListener { btn ->
                btn.isEnabled = false
                BmUpdateUi.check(this, silent = false) { btn.isEnabled = true }
            }
        } catch (t: Throwable) {
            reportFatal("设置页初始化失败", t)
            finish()
        }
    }

    /**
     * 沉浸式主题：状态栏与标题栏同色、Toolbar 延伸到状态栏后面
     */
    private fun applyImmersiveTheme(color: Int) {
        // 状态栏颜色与标题栏完全一致
        window.statusBarColor = color
        window.decorView.systemUiVisibility = (
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                or if (isVeryLight(color)) View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR else 0
            )

        binding.settingsToolbar.setBackgroundColor(color)
        val onColor = if (isVeryLight(color)) Color.BLACK else Color.WHITE
        binding.settingsToolbar.setTitleTextColor(onColor)
        binding.settingsToolbar.setNavigationIconTint(onColor)

        // Toolbar 高度 = actionBarSize + 状态栏高度，顶部留出状态栏空间（与主界面同款）
        val sbHeight = Resources.getSystem().getIdentifier(
            "status_bar_height", "dimen", "android"
        ).let { if (it > 0) Resources.getSystem().getDimensionPixelSize(it) else 0 }
        val actionBarSize = TypedValue().let { tv ->
            if (theme.resolveAttribute(android.R.attr.actionBarSize, tv, true)) {
                TypedValue.complexToDimensionPixelSize(tv.data, resources.displayMetrics)
            } else {
                (56 * resources.displayMetrics.density).toInt()
            }
        }
        binding.settingsToolbar.layoutParams.height = actionBarSize + sbHeight
        binding.settingsToolbar.setPadding(0, sbHeight, 0, 0)
    }

    /** 底部按钮避让导航栏（Android 15 边缘到边缘） */
    private fun applyBottomInset() {
        ViewCompat.setOnApplyWindowInsetsListener(binding.root) { _, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars())
            binding.btnBar.updatePadding(
                bottom = bars.bottom + (12 * resources.displayMetrics.density).toInt()
            )
            insets
        }
        ViewCompat.requestApplyInsets(binding.root)
    }

    /** 判断颜色是否接近白色（亮度 > 0.85），用于决定文字/图标深浅 */
    private fun isVeryLight(color: Int): Boolean {
        val r = Color.red(color)
        val g = Color.green(color)
        val b = Color.blue(color)
        return (0.299 * r + 0.587 * g + 0.114 * b) / 255.0 > 0.85
    }

    /** 崩溃取证：写入应用私有目录 crash_settings.txt，同时 Toast 提示异常类型 */
    private fun reportFatal(what: String, t: Throwable) {
        try {
            val ts = java.text.SimpleDateFormat("MM-dd HH:mm:ss", java.util.Locale.US)
                .format(java.util.Date())
            java.io.File(filesDir, "crash_settings.txt").appendText(
                "[$ts] $what\n${android.util.Log.getStackTraceString(t)}\n\n"
            )
            android.util.Log.e("SettingsActivity", what, t)
        } catch (_: Throwable) {
        }
        Toast.makeText(this, "$what: ${t.javaClass.simpleName}", Toast.LENGTH_LONG).show()
    }

    /** 把配置写入滑块/开关（UI 预览） */
    private fun bindFromConfig(cfg: BmConfig) {
        binding.sliderExamPoll.value = cfg.examPollMs.toFloat()
        binding.sliderAnswerDelay.value = cfg.answerDelayMs.toFloat()
        binding.sliderLock.value = cfg.lockMs.toFloat()
        binding.sliderTotalQuestions.value = cfg.totalQuestions.toFloat()
        binding.sliderPointsPoll.value = cfg.pointsPollMs.toFloat()
        binding.switchAutoSubmit.isChecked = cfg.autoSubmit
        binding.switchAutoQCount.isChecked = cfg.autoQuestionCount
        refreshValueLabels()
        updateTotalQuestionsEnabled()
    }

    /** 自动题数开关联动：开启时总题数滑块+数值置灰失效 */
    private fun updateTotalQuestionsEnabled() {
        val auto = binding.switchAutoQCount.isChecked
        binding.sliderTotalQuestions.isEnabled = !auto
        binding.valueTotalQuestions.alpha = if (auto) 0.4f else 1f
    }

    private fun setupListeners() {
        binding.sliderExamPoll.addOnChangeListener { _, _, _ -> refreshValueLabels() }
        binding.sliderAnswerDelay.addOnChangeListener { _, _, _ -> refreshValueLabels() }
        binding.sliderLock.addOnChangeListener { _, _, _ -> refreshValueLabels() }
        binding.sliderTotalQuestions.addOnChangeListener { _, _, _ -> refreshValueLabels() }
        binding.sliderPointsPoll.addOnChangeListener { _, _, _ -> refreshValueLabels() }
        binding.switchAutoQCount.setOnCheckedChangeListener { _, _ -> updateTotalQuestionsEnabled() }

        // 恢复默认：还原 6 项默认值并立即保存下发
        binding.btnRestoreDefault.setOnClickListener {
            settings.save(BmConfig.DEFAULT)
            bindFromConfig(BmConfig.DEFAULT)
            pushResult()
            Toast.makeText(this, R.string.toast_default, Toast.LENGTH_SHORT).show()
        }

        // 保存并应用
        binding.btnSaveApply.setOnClickListener {
            val cfg = collectConfig()
            settings.save(cfg)
            pushResult()
            Toast.makeText(this, R.string.toast_saved, Toast.LENGTH_SHORT).show()
            finish()
        }
    }

    /** 从 UI 收集配置（滑块步进已保证对齐范围，仍做归一化兜底） */
    private fun collectConfig(): BmConfig = BmConfig.normalize(
        examPollMs = binding.sliderExamPoll.value.toInt(),
        answerDelayMs = binding.sliderAnswerDelay.value.toInt(),
        lockMs = binding.sliderLock.value.toInt(),
        pointsPollMs = binding.sliderPointsPoll.value.toInt(),
        totalQuestions = binding.sliderTotalQuestions.value.toInt(),
        autoSubmit = binding.switchAutoSubmit.isChecked,
        autoQuestionCount = binding.switchAutoQCount.isChecked
    )

    /** 通知 MainActivity 立即应用最新配置 */
    private fun pushResult() {
        setResult(RESULT_OK)
    }

    private fun refreshValueLabels() {
        binding.valueExamPoll.text = getString(R.string.fmt_ms, binding.sliderExamPoll.value.toInt())
        binding.valueAnswerDelay.text = getString(R.string.fmt_ms, binding.sliderAnswerDelay.value.toInt())
        binding.valueLock.text = getString(R.string.fmt_ms, binding.sliderLock.value.toInt())
        binding.valueTotalQuestions.text = getString(R.string.fmt_questions, binding.sliderTotalQuestions.value.toInt())
        binding.valuePointsPoll.text = getString(R.string.fmt_ms, binding.sliderPointsPoll.value.toInt())
    }

    companion object {
        /** MainActivity → SettingsActivity 传递的标题栏颜色 extra */
        const val EXTRA_TOOLBAR_COLOR = "extra_toolbar_color"
    }
}