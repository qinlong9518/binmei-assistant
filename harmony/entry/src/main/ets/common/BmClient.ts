// 彬煤站点协议（HarmonyOS ArkTS 实现，与桌面/Android 端同源算法）
import http from '@ohos.net.http';

const BASE = 'http://61.185.41.209:8888';

// Esdt 算法：逐字符码点拼接 + '^' + 各码点长度表
export function esdt(code: string): string {
  let c = '';
  let l: string[] = [];
  for (let i = 0; i < code.length; i++) {
    const t = code.charCodeAt(i).toString();
    l.push(String(t.length));
    c += t;
  }
  return c + '^' + l.join(',');
}

// JS escape() 等价实现
export function jsEscape(s: string): string {
  let sb = '';
  for (let i = 0; i < s.length; i++) {
    const ch = s[i];
    const c = s.charCodeAt(i);
    const isAlnum = /[a-zA-Z0-9]/.test(ch);
    if (isAlnum || '@*_+-./'.includes(ch)) {
      sb += ch;
    } else if (c < 256) {
      sb += '%' + c.toString(16).toUpperCase().padStart(2, '0');
    } else {
      sb += '%u' + c.toString(16).toUpperCase().padStart(4, '0');
    }
  }
  return sb;
}

export function formEscape(raw: string): string {
  return jsEscape(esdt(raw));
}

// PointsDetail 积分明细
export interface PointsDetail {
  Name: string;
  Cur: number;
  Max: number;
}

// Client 协议客户端
export class BmClient {
  private cookies: string = '';
  account: string = '';
  pid: string = '';
  name: string = '';
  examTypeId: string = '';

  private async request(method: string, url: string, extraHeaders: Record<string, string>,
                        body: string): Promise<string> {
    const h = http.createHttp();
    try {
      const headers: Record<string, string> = {
        'User-Agent': 'Mozilla/5.0 (Linux; Android 15) AppleWebKit/537.36 Mobile Safari/537.36',
        'X-Requested-With': 'XMLHttpRequest',
        ...extraHeaders
      };
      if (this.cookies) {
        headers['Cookie'] = this.cookies;
      }
      if (method === 'POST') {
        headers['Content-Type'] = 'application/x-www-form-urlencoded; charset=UTF-8';
      }
      const resp = await h.request(BASE + url, {
        method: method === 'POST' ? http.RequestMethod.POST : http.RequestMethod.GET,
        header: headers,
        extraData: body || undefined,
        connectTimeout: 15000,
        readTimeout: 20000
      });
      const setCookie = resp.header['set-cookie'] || resp.header['Set-Cookie'];
      if (setCookie) {
        const sc = Array.isArray(setCookie) ? setCookie.join('; ') : setCookie;
        this.mergeCookies(sc);
      }
      return resp.result as string;
    } finally {
      h.destroy();
    }
  }

  private mergeCookies(setCookie: string) {
    // 简单合并：解析 name=value
    const parts = setCookie.split(/,(?=[^ ;]+=)/);
    const map: Record<string, string> = {};
    // 已有
    if (this.cookies) {
      this.cookies.split('; ').forEach(kv => {
        const i = kv.indexOf('=');
        if (i > 0) map[kv.slice(0, i)] = kv.slice(i + 1);
      });
    }
    parts.forEach(p => {
      const seg = p.split(';')[0].trim();
      const i = seg.indexOf('=');
      if (i > 0) map[seg.slice(0, i)] = seg.slice(i + 1);
    });
    this.cookies = Object.keys(map).map(k => `${k}=${map[k]}`).join('; ');
  }

  async tryLogin(account: string, password: string, yzm: string): Promise<string | null> {
    const body = `idcard=${formEscape(account)}&openid=&yzm=${yzm}&pwd=${formEscape(password)}&style=0&auto=true`;
    const resp = await this.request('POST', '/PersonWap/GetPersonInfo', {}, body);
    const strs = resp.split('|');
    if (strs[0] === '') {
      this.pid = strs[1];
      this.name = (strs[2] || '').trim();
      return null;
    }
    return strs[0];
  }

  async login(account: string, password: string): Promise<boolean> {
    let err = await this.tryLogin(account, password, '1');
    if (err) {
      err = await this.tryLogin(account, password, 'auto');
    }
    if (!err) {
      this.account = account;
      // 建立服务端会话（原始 pid）
      const form = `pid=${this.pid}&wx=`;
      await this.request('POST', '/PersonWap/FirstIndexOne', {}, form);
      return true;
    }
    return false;
  }

  async getPoints(): Promise<PointsDetail[]> {
    const e = jsEscape(esdt(this.pid));
    const resp = await this.request(
      'GET', `/AccumulateManger/S_Accumulate/GetPersonTodayAccumulateOne?pid=${encodeURIComponent(e)}`, {}, '');
    const j = JSON.parse(resp);
    const out: PointsDetail[] = [];
    if (j.success && Array.isArray(j.data)) {
      for (const d of j.data) {
        out.push({ Name: d.AccumulateName, Cur: d.CurAccumulate, Max: d.MaxAccumulate });
      }
    }
    return out;
  }
}