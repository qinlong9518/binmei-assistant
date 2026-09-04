package com.bm365.app

import android.content.Context
import android.content.SharedPreferences
import android.webkit.CookieManager

/**
 * 账号管理：保存的账号列表 + 最近登录账号。
 * 密码按需求全部账号统一为内置默认值，验证码默认 "1"。
 */
class BmAccountManager(context: Context) {

    private val prefs: SharedPreferences =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    /** 是否已有登录账号 */
    fun isLoggedIn(): Boolean = prefs.getString(KEY_LAST_ACCOUNT, null)?.isNotBlank() == true

    /** 最近登录账号（未登录返回 null） */
    fun lastAccount(): String? =
        prefs.getString(KEY_LAST_ACCOUNT, null)?.takeIf { it.isNotBlank() }

    /** 保存的账号列表（最近登录的排最前） */
    fun getAccounts(): List<String> {
        val raw = prefs.getString(KEY_ACCOUNTS, null) ?: return emptyList()
        return try {
            val arr = org.json.JSONArray(raw)
            (0 until arr.length()).mapNotNull { i ->
                arr.optString(i).takeIf { it.isNotBlank() }
            }
        } catch (e: Exception) {
            emptyList()
        }
    }

    /** 登录成功后调用：记录账号（去重置顶），并保存账号对应的姓名（供界面显示人名） */
    fun onLoginSuccess(account: String, name: String? = null) {
        val list = getAccounts().toMutableList()
        list.remove(account)
        list.add(0, account)
        val editor = prefs.edit()
            .putString(KEY_ACCOUNTS, org.json.JSONArray(list).toString())
            .putString(KEY_LAST_ACCOUNT, account)
        if (!name.isNullOrBlank()) {
            editor.putString(KEY_NAME_PREFIX + account, name)
        }
        editor.apply()
    }

    /** 账号显示名：有姓名映射显示人名，否则回落显示账号本身 */
    fun displayName(account: String): String =
        prefs.getString(KEY_NAME_PREFIX + account, null)?.takeIf { it.isNotBlank() } ?: account

    /** 退出登录：仅清除"最近登录"标记，账号列表保留（下拉可快速重选） */
    fun logout() {
        prefs.edit().remove(KEY_LAST_ACCOUNT).apply()
    }

    companion object {
        const val PREFS_NAME = "bm_accounts"
        const val KEY_ACCOUNTS = "accounts_json"
        const val KEY_LAST_ACCOUNT = "last_account"

        /** 姓名 映射键前缀：name_<账号> */
        const val KEY_NAME_PREFIX = "name_"

        /** 全部账号统一默认密码（按业务需求内置） */
        const val DEFAULT_PASSWORD = "Ydmk12345.6"

        /** 默认验证码 */
        const val DEFAULT_YZM = "1"

        /** 清除站点全部 Cookie（退出/切换账号时） */
        fun clearSiteCookies() {
            CookieManager.getInstance().apply {
                removeAllCookies(null)
                flush()
            }
        }
    }
}