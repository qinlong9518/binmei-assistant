package com.bm365.app

import android.util.Log
import androidx.lifecycle.LiveData
import androidx.lifecycle.MutableLiveData
import androidx.lifecycle.ViewModel
import org.json.JSONArray
import org.json.JSONObject

/**
 * 积分 ViewModel：解析隐藏 WebView 传来的 JSON，管理积分列表状态
 */
class PointsViewModel : ViewModel() {

    companion object {
        private const val TAG = "PointsViewModel"
    }

    /** 积分任务列表 */
    private val _pointsList = MutableLiveData<List<PointsItem>>(emptyList())
    val pointsList: LiveData<List<PointsItem>> = _pointsList

    /** 今日总积分 */
    private val _totalPoints = MutableLiveData(0)
    val totalPoints: LiveData<Int> = _totalPoints

    /** 状态文本（加载中 / 错误 / 请登录） */
    private val _statusText = MutableLiveData("积分加载中...")
    val statusText: LiveData<String> = _statusText

    /** 列表是否为空 */
    private val _isEmpty = MutableLiveData(true)
    val isEmpty: LiveData<Boolean> = _isEmpty

    /**
     * 解析积分 JSON 数据
     * 期望格式: { "success": true, "data": [ { "AccumulateName": "...", "CurAccumulate": N, "MaxAccumulate": N, "AccumulateRule": "..." }, ... ] }
     * 或直接是数组: [ { ... }, ... ]
     */
    fun parsePointsJson(jsonString: String) {
        try {
            val items = mutableListOf<PointsItem>()
            val jsonArray: JSONArray

            val trimmed = jsonString.trim()
            if (trimmed.startsWith("{")) {
                // 可能是有 success/data 包裹的对象
                val root = JSONObject(trimmed)
                if (root.has("data")) {
                    val dataValue = root.get("data")
                    jsonArray = if (dataValue is JSONArray) {
                        dataValue
                    } else {
                        Log.w(TAG, "data 字段不是数组: $dataValue")
                        _statusText.postValue("积分数据格式异常")
                        return
                    }
                } else {
                    // 也可能是纯数组被包在 JSON.stringify 里？不会，stringify 返回字符串
                    Log.w(TAG, "未知 JSON 对象格式: $trimmed")
                    _statusText.postValue("积分数据格式异常")
                    return
                }
            } else if (trimmed.startsWith("[")) {
                jsonArray = JSONArray(trimmed)
            } else {
                Log.w(TAG, "无法解析的 JSON: ${trimmed.take(100)}")
                _statusText.postValue("积分数据格式异常")
                return
            }

            for (i in 0 until jsonArray.length()) {
                val obj = jsonArray.getJSONObject(i)
                items.add(
                    PointsItem(
                        name = obj.optString("AccumulateName", "未知任务"),
                        rule = obj.optString("AccumulateRule", ""),
                        current = obj.optInt("CurAccumulate", 0),
                        max = obj.optInt("MaxAccumulate", 0)
                    )
                )
            }

            val total = items.sumOf { it.current }

            _pointsList.postValue(items)
            _totalPoints.postValue(total)
            _isEmpty.postValue(items.isEmpty())
            _statusText.postValue(if (items.isEmpty()) "暂无积分数据" else "")

            Log.d(TAG, "解析完成: ${items.size} 项积分, 总计 $total 分")
        } catch (e: Exception) {
            Log.e(TAG, "JSON 解析失败", e)
            _statusText.postValue("数据解析失败: ${e.message}")
        }
    }

    fun setError(message: String) {
        _statusText.postValue(message)
    }
}
