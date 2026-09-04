package com.bm365.app

import android.app.AlertDialog
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.widget.ProgressBar
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.FileProvider
import org.json.JSONObject
import java.io.File
import java.net.HttpURLConnection
import java.net.URL

/**
 * 在线更新：GitHub 托管（无需自有服务器）。
 *
 * 发布结构：
 *   仓库根目录/update.json              —— 版本元数据（raw / jsDelivr CDN 多源）
 *   Releases → 资产 app-release.apk     —— 安装包（直链 + gh-proxy 镜像回退）
 *
 * update.json 格式：
 * {
 *   "versionCode": 6, "versionName": "1.5", "force": false,
 *   "changelog": "1. xxx\n2. yyy",
 *   "apkUrl": "https://github.com/<user>/<repo>/releases/download/v1.5/app-release.apk",
 *   "apkFile": "app-release.apk"
 * }
 */
object BmUpdate {

    // ================== GitHub 更新源（已绑定：qinlong9518/binmei-assistant） ==================
    const val REPO_OWNER = "qinlong9518"
    const val REPO_NAME = "binmei-assistant"
    const val BRANCH = "main"
    // ================================================================

    private fun metaUrls(): List<String> {
        // cache-buster：时间戳参数让各 CDN 视为新请求，绕过 12~24h 缓存
        val ts = System.currentTimeMillis() / 60000 // 分钟级，同分钟内可命中本地 HTTP 缓存
        return listOf(
            // 镜像源（国内可达性优先，免翻墙）
            "https://gh-proxy.com/https://raw.githubusercontent.com/$REPO_OWNER/$REPO_NAME/$BRANCH/update.json?_t=$ts",
            // 原生源
            "https://raw.githubusercontent.com/$REPO_OWNER/$REPO_NAME/$BRANCH/update.json?_t=$ts",
            "https://cdn.jsdelivr.net/gh/$REPO_OWNER/$REPO_NAME@$BRANCH/update.json?_t=$ts",
            "https://fastly.jsdelivr.net/gh/$REPO_OWNER/$REPO_NAME@$BRANCH/update.json?_t=$ts"
        )
    }

    /** 远端版本元数据 */
    data class Meta(
        val versionCode: Int,
        val versionName: String,
        val changelog: String,
        val force: Boolean,
        val apkUrl: String,
        val apkFile: String
    )

    /** 当前版本码 */
    fun currentVersionCode(ctx: Context): Int =
        ctx.packageManager.getPackageInfo(ctx.packageName, 0).let {
            if (Build.VERSION.SDK_INT >= 28) it.longVersionCode.toInt() else it.versionCode
        }

    /** 当前版本名 */
    fun currentVersionName(ctx: Context): String =
        ctx.packageManager.getPackageInfo(ctx.packageName, 0).versionName ?: ""

    /** 拉取远端元数据（多源回退）。网络操作，须子线程调用；失败返回 null */
    fun fetchMeta(): Meta? {
        for (u in metaUrls()) {
            try {
                val text = httpGet(u, 5000)
                if (text.isNotBlank()) return parseMeta(text)
            } catch (_: Exception) {
            }
        }
        return null
    }

    private fun parseMeta(json: String): Meta {
        val o = JSONObject(json)
        return Meta(
            versionCode = o.optInt("versionCode", 0),
            versionName = o.optString("versionName", ""),
            changelog = o.optString("changelog", "").replace("\\n", "\n"),
            force = o.optBoolean("force", false),
            apkUrl = o.optString("apkUrl", ""),
            apkFile = o.optString("apkFile", "app-release.apk")
        )
    }

    /**
     * 下载 APK（直链 + 镜像回退，进度回调百分比）。
     * 网络操作，须子线程调用；全部失败返回 null。
     */
    fun downloadApk(ctx: Context, meta: Meta, onProgress: (Int) -> Unit): File? {
        if (meta.apkUrl.isBlank()) return null
        val dir = File(ctx.getExternalFilesDir(null), "update").apply { mkdirs() }
        // 清理旧安装包，避免堆积
        dir.listFiles()?.forEach { it.delete() }
        val dest = File(dir, meta.apkFile)
        val candidates = listOf(
            // 国内镜像优先（实测 200，1~2s 内开始传输；github 直链在国内 DNS 下常挂起，放最后）
            "https://ghfast.top/${meta.apkUrl}",
            "https://ghproxy.net/${meta.apkUrl}",
            "https://gh-proxy.com/${meta.apkUrl}",
            meta.apkUrl
        )
        for (u in candidates) {
            try {
                if (download(u, dest, onProgress)) return dest
            } catch (_: Exception) {
            }
        }
        return null
    }

    private fun download(url: String, dest: File, onProgress: (Int) -> Unit): Boolean {
        var conn: HttpURLConnection? = null
        return try {
            conn = (URL(url).openConnection() as HttpURLConnection).apply {
                // 注意：connectTimeout 不含 DNS 解析时间，被污染的域名会额外挂起；
                // 因此超时收紧，且把国内可达性差的源排在候选末尾
                connectTimeout = 5000
                readTimeout = 15000
                instanceFollowRedirects = true
                setRequestProperty("User-Agent", "BM365App/1.0")
            }
            if (conn.responseCode !in 200..299) return false
            val total = conn.contentLengthLong
            conn.inputStream.use { input ->
                dest.outputStream().use { out ->
                    val buf = ByteArray(64 * 1024)
                    var done = 0L
                    var lastPct = -1
                    while (true) {
                        val r = input.read(buf)
                        if (r == -1) break
                        out.write(buf, 0, r)
                        done += r
                        if (total > 0) {
                            val pct = (done * 100 / total).toInt()
                            if (pct != lastPct) {
                                lastPct = pct
                                onProgress(pct)
                            }
                        }
                    }
                }
            }
            dest.length() > 1024 * 1024 // 有效 APK 至少 1MB
        } finally {
            conn?.disconnect()
        }
    }

    /** 拉起系统安装器 */
    fun install(ctx: Context, apk: File) {
        val uri = FileProvider.getUriForFile(ctx, "${ctx.packageName}.fileprovider", apk)
        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        ctx.startActivity(intent)
    }

    /** 是否已授予"安装未知应用"权限 */
    fun canInstall(ctx: Context): Boolean =
        Build.VERSION.SDK_INT < 26 || ctx.packageManager.canRequestPackageInstalls()

    /** 跳转"安装未知应用"授权页 */
    fun requestInstallPermission(ctx: Context) {
        if (Build.VERSION.SDK_INT >= 26) {
            ctx.startActivity(
                Intent(
                    android.provider.Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                    Uri.parse("package:${ctx.packageName}")
                ).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            )
        }
    }

    private fun httpGet(url: String, timeoutMs: Int): String {
        var conn: HttpURLConnection? = null
        return try {
            conn = (URL(url).openConnection() as HttpURLConnection).apply {
                connectTimeout = timeoutMs
                readTimeout = timeoutMs
                setRequestProperty("User-Agent", "BM365App/1.0")
            }
            if (conn.responseCode in 200..299)
                conn.inputStream.bufferedReader(Charsets.UTF_8).readText()
            else ""
        } finally {
            conn?.disconnect()
        }
    }
}

/**
 * 更新交互 UI：设置页显式检查 / 主界面静默检查共用。
 */
object BmUpdateUi {

    /** 检查并按结果提示。silent=true 时仅在有新版本时打扰用户 */
    fun check(activity: AppCompatActivity, silent: Boolean, onComplete: (() -> Unit)? = null) {
        val act = activity
        Thread {
            val meta = try {
                BmUpdate.fetchMeta()
            } catch (_: Exception) {
                null
            }
            act.runOnUiThread {
                onComplete?.invoke()
                when {
                    meta == null ->
                        if (!silent) Toast.makeText(act, "检查更新失败：网络不可达", Toast.LENGTH_SHORT).show()

                    meta.versionCode <= BmUpdate.currentVersionCode(act) ->
                        if (!silent) Toast.makeText(
                            act, "已是最新版本 v${BmUpdate.currentVersionName(act)}", Toast.LENGTH_SHORT
                        ).show()

                    else -> showUpdateDialog(act, meta)
                }
            }
        }.start()
    }

    private fun showUpdateDialog(act: AppCompatActivity, meta: BmUpdate.Meta) {
        val builder = AlertDialog.Builder(act)
            .setTitle("发现新版本 v${meta.versionName}")
            .setMessage("更新内容：\n${meta.changelog}\n\n当前版本：v${BmUpdate.currentVersionName(act)}")
            .setPositiveButton("立即更新") { _, _ -> startDownload(act, meta) }
            .setCancelable(!meta.force)
        if (!meta.force) builder.setNegativeButton("稍后", null)
        builder.show()
    }

    private fun startDownload(act: AppCompatActivity, meta: BmUpdate.Meta) {
        val progress = ProgressBar(act, null, android.R.attr.progressBarStyleHorizontal)
        val dialog = AlertDialog.Builder(act)
            .setTitle("下载中 0%")
            .setView(progress)
            .setCancelable(false)
            .create()
        dialog.show()

        Thread {
            val file = BmUpdate.downloadApk(act, meta) { pct ->
                act.runOnUiThread {
                    dialog.setTitle("下载中 $pct%")
                    progress.progress = pct
                }
            }
            act.runOnUiThread {
                dialog.dismiss()
                when {
                    file == null ->
                        Toast.makeText(act, "下载失败，请检查网络后重试", Toast.LENGTH_SHORT).show()

                    BmUpdate.canInstall(act) ->
                        BmUpdate.install(act, file)

                    else -> {
                        Toast.makeText(act, "请先授权「安装未知应用」", Toast.LENGTH_LONG).show()
                        BmUpdate.requestInstallPermission(act)
                    }
                }
            }
        }.start()
    }
}