package com.bm365.app

import android.webkit.CookieManager
import java.net.HttpURLConnection
import java.net.URL

/**
 * 彬煤站点原生登录：完整复刻站点登录流程。
 *
 * 流程（与站点 LoginWap.js / 首页自动登录一致）：
 * 1. POST /PersonWap/GetPersonInfo  {idcard: Esdt(账号), openid:'', yzm, pwd: Esdt(密码), style:0, auto:'true'}
 * 2. 响应按 '|' 分割：strs[0]=='' 表示成功，strs[1] 为 pid
 * 3. 成功后写 Cookie：xxidnumber / xxpid / xxidpwd / autoLogin（escape 编码、path=/）
 * 4. 服务端返回的 Set-Cookie 同步写入全局 CookieManager（会话绑定）
 *
 * Cookie 全局共享后，主/隐藏两个 WebView 加载 /PersonWap/Index0018 即为已登录态，
 * 积分模块（隐藏 WebView 的 M_PersonId 环境）随之自动可用。
 */
object BmHttpLogin {

    private const val BASE = "http://61.185.41.209:8888"
    private const val LOGIN_API = "$BASE/PersonWap/GetPersonInfo"

    /** 登录结果 */
    data class Result(val success: Boolean, val pid: String = "", val message: String = "")

    /**
     * 登录（网络请求，须在子线程调用）。
     * 验证码策略：先传 DEFAULT_YZM("1")，若站点校验不通过则回退 yzm=auto（站点首页自动登录语义）。
     */
    fun login(account: String, password: String = BmAccountManager.DEFAULT_PASSWORD): Result {
        val first = doLogin(account, password, BmAccountManager.DEFAULT_YZM)
        return if (first.success) first else doLogin(account, password, "auto")
    }

    /** 单次登录请求 */
    private fun doLogin(account: String, password: String, yzm: String): Result {
        var conn: HttpURLConnection? = null
        return try {
            val body = "idcard=${jsEscape(esdtRaw(account))}" +
                    "&openid=" +
                    "&yzm=$yzm" +
                    "&pwd=${jsEscape(esdtRaw(password))}" +
                    "&style=0&auto=true"

            conn = (URL(LOGIN_API).openConnection() as HttpURLConnection).apply {
                requestMethod = "POST"
                doOutput = true
                connectTimeout = 12000
                readTimeout = 12000
                setRequestProperty("Content-Type", "application/x-www-form-urlencoded")
                setRequestProperty(
                    "User-Agent",
                    "Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.77 Mobile Safari/537.36"
                )
            }
            conn.outputStream.use { it.write(body.toByteArray(Charsets.UTF_8)) }

            val text = conn.inputStream.bufferedReader(Charsets.UTF_8).readText()
            val strs = text.split("|")

            if (strs.firstOrNull()?.isEmpty() == true) {
                val pid = strs.getOrElse(1) { "" }
                // 1) 服务端 Set-Cookie 同步进全局 CookieManager（会话态）
                val setCookies = conn.headerFields?.get("Set-Cookie")
                if (setCookies != null) {
                    for (sc in setCookies) {
                        CookieManager.getInstance().setCookie("$BASE/", sc)
                    }
                }
                // 2) 复刻站点登录成功逻辑写身份 Cookie（escape 值 + path=/）
                writeIdentityCookies(account, pid, password)
                CookieManager.getInstance().flush()
                Result(true, pid = pid)
            } else {
                Result(false, message = strs.firstOrNull() ?: "登录失败")
            }
        } catch (e: Exception) {
            Result(false, message = "网络异常: ${e.message ?: e.javaClass.simpleName}")
        } finally {
            conn?.disconnect()
        }
    }

    /** 写站点身份 Cookie（与站点 setCookie('xxidnumber'/...) 等价，显式 path=/ 更稳） */
    private fun writeIdentityCookies(account: String, pid: String, password: String) {
        val cm = CookieManager.getInstance()
        cm.setCookie("$BASE/", "xxidnumber=${jsEscape(account)}; path=/")
        cm.setCookie("$BASE/", "xxpid=${jsEscape(pid)}; path=/")
        cm.setCookie("$BASE/", "xxidpwd=${jsEscape(password)}; path=/")
        cm.setCookie("$BASE/", "autoLogin=true; path=/")
    }

    /** Esdt：字符码点拼接 + 各码点长度列表（算法取自站点 JS，逐字对应） */
    private fun esdtRaw(code: String): String {
        val c = StringBuilder()
        val l = ArrayList<String>(code.length)
        for (ch in code) {
            val t = ch.code
            l.add(t.toString().length.toString())
            c.append(t)
        }
        return c.toString() + "^" + l.joinToString(",")
    }

    /** JS escape() 的 Kotlin 等价实现（字母数字与 @*_+-./ 不编码，ASCII 用 %XX，非 ASCII 用 %uXXXX） */
    private fun jsEscape(s: String): String {
        val sb = StringBuilder(s.length + 16)
        for (ch in s) {
            val c = ch.code
            when {
                ch.isLetterOrDigit() || ch in "@*_+-./" -> sb.append(ch)
                c < 256 -> sb.append('%').append(String.format("%02X", c))
                else -> sb.append("%u").append(String.format("%04X", c))
            }
        }
        return sb.toString()
    }
}