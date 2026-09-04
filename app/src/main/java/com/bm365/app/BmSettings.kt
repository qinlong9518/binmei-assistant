package com.bm365.app

import android.content.Context
import android.content.SharedPreferences

/**
 * 可调配置项（与设置页 6 项一一对应）
 *
 * @param examPollMs     答题主循环轮询间隔（10–100ms）
 * @param answerDelayMs  答题后等待（点击下一题前延时，10–100ms）
 * @param lockMs         动作锁定保持时长（10–100ms）
 * @param pointsPollMs   积分轮询间隔（1000–5000ms）
 * @param totalQuestions 总题数（自动交卷阈值，1–200）
 * @param autoSubmit     自动交卷开关
 * @param autoQuestionCount 自动题数开关：开启时按试卷下发数量作答，totalQuestions 失效
 * @param autoStartPollMs 自动开考调度脚本轮询间隔（1000–5000ms）
 */
data class BmConfig(
    val examPollMs: Int = DEFAULT.examPollMs,
    val answerDelayMs: Int = DEFAULT.answerDelayMs,
    val lockMs: Int = DEFAULT.lockMs,
    val pointsPollMs: Int = DEFAULT.pointsPollMs,
    val totalQuestions: Int = DEFAULT.totalQuestions,
    val autoSubmit: Boolean = DEFAULT.autoSubmit,
    val autoQuestionCount: Boolean = DEFAULT.autoQuestionCount,
    val autoStartPollMs: Int = DEFAULT.autoStartPollMs
) {

    /** 序列化为注入 JS 的 window.BM_CFG 对象字面量（无引号包裹，可直接 eval） */
    fun toJson(): String =
        "{\"examPollMs\":$examPollMs" +
        ",\"answerDelayMs\":$answerDelayMs" +
        ",\"lockMs\":$lockMs" +
        ",\"pointsPollMs\":$pointsPollMs" +
        ",\"totalQuestions\":$totalQuestions" +
        ",\"autoSubmit\":$autoSubmit" +
        ",\"autoQuestionCount\":$autoQuestionCount" +
        ",\"autoStartPollMs\":$autoStartPollMs}"

    companion object {
        /** 全部默认值（"恢复默认"按钮的目标状态） */
        val DEFAULT = BmConfig(
            examPollMs = 50,
            answerDelayMs = 50,
            lockMs = 80,
            pointsPollMs = 5000,
            totalQuestions = 40,
            autoSubmit = true,
            autoQuestionCount = false,
            autoStartPollMs = 3000
        )

        // 取值范围（与设置页滑块范围一致）
        val RANGE_EXAM_POLL = 10L..100L
        val RANGE_ANSWER_DELAY = 10L..100L
        val RANGE_LOCK = 10L..100L
        val RANGE_POINTS_POLL = 1000L..5000L
        val RANGE_TOTAL_QUESTIONS = 1L..200L
        val RANGE_AUTOSTART_POLL = 1000L..5000L

        /** 归一化：越界裁剪到合法范围，防止脏数据破坏注入脚本 */
        fun normalize(
            examPollMs: Int,
            answerDelayMs: Int,
            lockMs: Int,
            pointsPollMs: Int,
            totalQuestions: Int,
            autoSubmit: Boolean,
            autoQuestionCount: Boolean = false,
            autoStartPollMs: Int = DEFAULT.autoStartPollMs
        ): BmConfig = BmConfig(
            examPollMs = examPollMs.coerceIn(RANGE_EXAM_POLL.first.toInt(), RANGE_EXAM_POLL.last.toInt()),
            answerDelayMs = answerDelayMs.coerceIn(RANGE_ANSWER_DELAY.first.toInt(), RANGE_ANSWER_DELAY.last.toInt()),
            lockMs = lockMs.coerceIn(RANGE_LOCK.first.toInt(), RANGE_LOCK.last.toInt()),
            pointsPollMs = pointsPollMs.coerceIn(RANGE_POINTS_POLL.first.toInt(), RANGE_POINTS_POLL.last.toInt()),
            totalQuestions = totalQuestions.coerceIn(RANGE_TOTAL_QUESTIONS.first.toInt(), RANGE_TOTAL_QUESTIONS.last.toInt()),
            autoSubmit = autoSubmit,
            autoQuestionCount = autoQuestionCount,
            autoStartPollMs = autoStartPollMs.coerceIn(RANGE_AUTOSTART_POLL.first.toInt(), RANGE_AUTOSTART_POLL.last.toInt())
        )
    }
}

/**
 * 设置持久化：SharedPreferences 文件 bm_settings
 * 键：exam_poll_ms / answer_delay_ms / lock_ms / points_poll_ms / total_questions / auto_submit
 */
class BmSettings(context: Context) {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    /** 读取配置（缺省回落到 DEFAULT，越界值自动裁剪） */
    fun load(): BmConfig = BmConfig.normalize(
        examPollMs = prefs.getInt(KEY_EXAM_POLL_MS, BmConfig.DEFAULT.examPollMs),
        answerDelayMs = prefs.getInt(KEY_ANSWER_DELAY_MS, BmConfig.DEFAULT.answerDelayMs),
        lockMs = prefs.getInt(KEY_LOCK_MS, BmConfig.DEFAULT.lockMs),
        pointsPollMs = prefs.getInt(KEY_POINTS_POLL_MS, BmConfig.DEFAULT.pointsPollMs),
        totalQuestions = prefs.getInt(KEY_TOTAL_QUESTIONS, BmConfig.DEFAULT.totalQuestions),
        autoSubmit = prefs.getBoolean(KEY_AUTO_SUBMIT, BmConfig.DEFAULT.autoSubmit),
        autoQuestionCount = prefs.getBoolean(KEY_AUTO_QCOUNT, BmConfig.DEFAULT.autoQuestionCount),
        autoStartPollMs = prefs.getInt(KEY_AUTO_START_POLL, BmConfig.DEFAULT.autoStartPollMs)
    )

    /** 保存配置 */
    fun save(cfg: BmConfig) {
        prefs.edit()
            .putInt(KEY_EXAM_POLL_MS, cfg.examPollMs)
            .putInt(KEY_ANSWER_DELAY_MS, cfg.answerDelayMs)
            .putInt(KEY_LOCK_MS, cfg.lockMs)
            .putInt(KEY_POINTS_POLL_MS, cfg.pointsPollMs)
            .putInt(KEY_TOTAL_QUESTIONS, cfg.totalQuestions)
            .putBoolean(KEY_AUTO_SUBMIT, cfg.autoSubmit)
            .putBoolean(KEY_AUTO_QCOUNT, cfg.autoQuestionCount)
            .putInt(KEY_AUTO_START_POLL, cfg.autoStartPollMs)
            .apply()
    }

    companion object {
        const val PREFS_NAME = "bm_settings"
        const val KEY_EXAM_POLL_MS = "exam_poll_ms"
        const val KEY_ANSWER_DELAY_MS = "answer_delay_ms"
        const val KEY_LOCK_MS = "lock_ms"
        const val KEY_POINTS_POLL_MS = "points_poll_ms"
        const val KEY_TOTAL_QUESTIONS = "total_questions"
        const val KEY_AUTO_SUBMIT = "auto_submit"
        const val KEY_AUTO_QCOUNT = "auto_question_count"
        const val KEY_AUTO_START_POLL = "auto_start_poll_ms"
    }
}
