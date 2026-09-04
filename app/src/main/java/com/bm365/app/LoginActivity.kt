package com.bm365.app

import android.content.Intent
import android.graphics.Color
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.PopupMenu
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity

/**
 * 登录页：QQ/微信风格原生界面。
 * 仅需输入账号；密码/验证码内置（BmAccountManager.DEFAULT_*）。
 * 支持下拉菜单快速切换历史账号。登录成功写入站点 Cookie（BmHttpLogin），
 * 主/隐藏两个 WebView 借助全局共享 Cookie 直接进入已登录的 Index0018，
 * 积分模块随之自动获取登录信息。
 */
class LoginActivity : AppCompatActivity() {

    private lateinit var etAccount: EditText
    private lateinit var btnLogin: Button
    private lateinit var tvError: TextView
    private lateinit var btnSwitchAccount: TextView
    private lateinit var accountManager: BmAccountManager

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        accountManager = BmAccountManager(this)

        // 已登录：直接进主界面（登录页仅首次启动或退出后出现）
        if (accountManager.isLoggedIn()) {
            startActivity(Intent(this, MainActivity::class.java))
            finish()
            return
        }

        setContentView(R.layout.activity_login)

        // 沉浸式：状态栏与渐变背景顶端同色（深绿）
        window.statusBarColor = Color.parseColor("#2E7D32")

        etAccount = findViewById(R.id.etAccount)
        btnLogin = findViewById(R.id.btnLogin)
        tvError = findViewById(R.id.tvError)
        btnSwitchAccount = findViewById(R.id.btnSwitchAccount)

        setupAccountDropdown()
        setupLoginButton()
    }

    /** 下拉按钮：弹出历史账号菜单，点选即填入 */
    private fun setupAccountDropdown() {
        val accounts = accountManager.getAccounts()
        if (accounts.isEmpty()) return
        btnSwitchAccount.visibility = View.VISIBLE
        accountManager.lastAccount()?.let { etAccount.setText(it) }

        btnSwitchAccount.setOnClickListener { anchor ->
            // 居中下拉弹窗：宽度随内容，水平居中于按钮
            val displayNames = accounts.map { accountManager.displayName(it) }
            val lpw = android.widget.ListPopupWindow(this)
            lpw.setAnchorView(anchor)
            lpw.setAdapter(
                android.widget.ArrayAdapter(this, android.R.layout.simple_list_item_1, displayNames)
            )
            lpw.setOnItemClickListener { _, _, pos, _ ->
                lpw.dismiss()
                val acc = accounts.getOrNull(pos)
                acc?.let {
                    etAccount.setText(it)
                    etAccount.setSelection(it.length)
                    tvError.visibility = View.GONE
                }
            }
            anchor.post {
                // 内容宽度（至少与按钮等宽），再水平居中：偏移 = (锚点宽 - 弹窗宽)/2
                val lvw = lpw.listView
                val contentW = (lvw?.measuredWidth ?: 0)
                    .coerceAtLeast(anchor.width)
                lpw.setContentWidth(contentW)
                lpw.horizontalOffset = ((anchor.width - contentW) / 2)
                lpw.show()
            }
        }
    }

    private fun setupLoginButton() {
        btnLogin.setOnClickListener {
            val account = etAccount.text.toString().trim()
            if (account.isEmpty()) {
                tvError.text = "请输入账号"
                tvError.visibility = View.VISIBLE
                return@setOnClickListener
            }
            startLogin(account)
        }
    }

    private fun startLogin(account: String) {
        btnLogin.isEnabled = false
        btnLogin.text = "登录中..."
        tvError.visibility = View.GONE

        Thread {
            val result = BmHttpLogin.login(account)
            runOnUiThread {
                btnLogin.isEnabled = true
                btnLogin.text = "登  录"
                if (result.success) {
                    accountManager.onLoginSuccess(account, result.name)
                    Toast.makeText(this, "登录成功", Toast.LENGTH_SHORT).show()
                    startActivity(Intent(this, MainActivity::class.java))
                    finish()
                } else {
                    tvError.text = result.message
                    tvError.visibility = View.VISIBLE
                }
            }
        }.start()
    }
}