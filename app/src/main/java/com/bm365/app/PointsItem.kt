package com.bm365.app

/**
 * 积分任务项数据模型
 * @param name 积分任务名称 (AccumulateName)
 * @param rule 积分规则描述 (AccumulateRule)
 * @param current 当前积分 (CurAccumulate)
 * @param max 积分上限 (MaxAccumulate)
 */
data class PointsItem(
    val name: String,
    val rule: String,
    val current: Int,
    val max: Int
) {
    /** 进度百分比 (0-100) */
    val progressPercent: Int
        get() = if (max > 0) ((current.toDouble() / max) * 100).toInt().coerceIn(0, 100) else 0
}
