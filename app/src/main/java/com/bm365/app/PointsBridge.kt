package com.bm365.app

import android.util.Log
import android.webkit.JavascriptInterface
import androidx.lifecycle.MutableLiveData

/**
 * JavaScript 桥接层：接收隐藏 WebView 中积分脚本回传的数据，
 * 通知原生层更新底部积分面板。
 */
class PointsBridge {

    companion object {
        private const val TAG = "PointsBridge"
    }

    /** 积分原始 JSON 字符串，供 DataModel 解析 */
    val pointsDataJson = MutableLiveData<String>()

    /** 错误信息 */
    val errorMessage = MutableLiveData<String>()

    /**
     * 积分脚本调用 Android.onPointsData(jsonString) 回传数据
     */
    @JavascriptInterface
    fun onPointsData(jsonString: String) {
        Log.d(TAG, "收到积分数据: ${jsonString.take(200)}...")
        pointsDataJson.postValue(jsonString)
    }

    /**
     * 积分脚本调用 Android.onError(message) 报告错误
     */
    @JavascriptInterface
    fun onError(message: String) {
        Log.w(TAG, "积分脚本错误: $message")
        errorMessage.postValue(message)
    }
}
