// 彬煤答题助手 · 鸿蒙主页面（ArkUI 声明式，微信绿主题）
import { BmClient, PointsDetail } from '../common/BmClient';

const GREEN = '#07C160';
const GREEN_DARK = '#06AD56';
const BG = '#F6F7F8';
const TEXT = '#1A1A1A';
const MUTED = '#888888';

@Entry
@Component
struct Index {
  @State account: string = '';
  @State userName: string = '';
  @State points: PointsDetail[] = [];
  @State loginStatus: string = '';
  @State isLoggedIn: boolean = false;
  @State running: boolean = false;
  @State logs: string[] = [];
  private client: BmClient = new BmClient();
  private timerId: number = -1;

  addLog(msg: string) {
    const t = new Date();
    const hh = t.getHours().toString().padStart(2, '0');
    const mm = t.getMinutes().toString().padStart(2, '0');
    const ss = t.getSeconds().toString().padStart(2, '0');
    this.logs.push(`${hh}:${mm}:${ss}  ${msg}`);
    if (this.logs.length > 200) {
      this.logs.shift();
    }
  }

  async doLogin() {
    const acc = this.account.trim();
    if (!acc) {
      this.loginStatus = '请输入账号';
      return;
    }
    this.loginStatus = '登录中…';
    try {
      const ok = await this.client.login(acc, 'Ydmk12345.6');
      if (ok) {
        this.userName = this.client.name;
        this.isLoggedIn = true;
        this.loginStatus = '';
        this.addLog(`✅ 登录成功: ${this.client.name}（${acc}）`);
        this.refreshPoints();
      } else {
        this.loginStatus = '登录失败：账号或密码错误';
      }
    } catch (e) {
      this.loginStatus = '登录失败：网络异常';
    }
  }

  async refreshPoints() {
    try {
      this.points = await this.client.getPoints();
    } catch (e) {
      // 忽略轮询错误
    }
  }

  startTask() {
    this.running = true;
    this.addLog('▶ 自动答题已启动（3秒轮询）');
    this.timerId = setInterval(() => {
      this.refreshPoints();
    }, 3000);
  }

  stopTask() {
    this.running = false;
    if (this.timerId >= 0) {
      clearInterval(this.timerId);
      this.timerId = -1;
    }
    this.addLog('⏹ 自动答题已停止');
  }

  pointColor(cur: number, max: number): string {
    return cur >= max ? GREEN : '#E6503C';
  }

  build() {
    Column() {
      if (!this.isLoggedIn) {
        // ===== 登录页 =====
        Column() {
          Blank().height(60)
          Text('彬煤答题助手')
            .fontSize(26).fontWeight(FontWeight.Bold).fontColor(TEXT)
          Text('学习积分自动管理')
            .fontSize(14).fontColor(MUTED).margin({ top: 8 })
          Blank().height(48)
          TextInput({ placeholder: '身份证账号', text: this.account })
            .width('82%').height(46)
            .backgroundColor('#FFFFFF')
            .borderRadius(8)
            .onChange((v) => { this.account = v; })
          Blank().height(16)
          Button('登 录')
            .width('82%').height(46)
            .backgroundColor(GREEN)
            .fontColor(Color.White)
            .fontSize(17)
            .borderRadius(8)
            .onClick(() => { this.doLogin(); })
          Blank().height(14)
          Text(this.loginStatus)
            .fontSize(13).fontColor('#E64340')
          Blank()
          Text('仅供学习交流使用')
            .fontSize(11).fontColor(MUTED).margin({ bottom: 20 })
        }
        .width('100%').height('100%')
        .backgroundColor(BG)
      } else {
        // ===== 主页面 =====
        Column() {
          // 绿色顶栏
          Row() {
            Text('彬煤答题助手')
              .fontSize(18).fontWeight(FontWeight.Bold).fontColor(Color.White)
            Blank()
            Text(`👤 ${this.userName}`)
              .fontSize(14).fontColor(Color.White)
          }
          .width('100%').padding({ left: 16, right: 16, top: 14, bottom: 14 })
          .backgroundColor(GREEN)

          Scroll() {
            Column() {
              // 积分网格
              Column() {
                Text('今日积分')
                  .fontSize(14).fontColor(MUTED).margin({ bottom: 10 })
                Flex({ wrap: FlexWrap.Wrap }) {
                  ForEach(this.points, (p: PointsDetail) => {
                    Column() {
                      Text(p.Name).fontSize(12).fontColor('#666666')
                      Text(`${p.Cur}/${p.Max}`)
                        .fontSize(17).fontWeight(FontWeight.Bold)
                        .fontColor(this.pointColor(p.Cur, p.Max))
                        .margin({ top: 4 })
                    }
                    .width('31%')
                    .padding(10)
                    .margin({ right: '1%', bottom: 8 })
                    .backgroundColor('#FFFFFF')
                    .borderRadius(10)
                    .border({ width: 1, color: '#E7E7E7' })
                  })
                }
                .width('100%')
              }
              .width('100%')
              .padding(14)
              .margin({ top: 12, left: 12, right: 12 })
              .backgroundColor('#FFFFFF')
              .borderRadius(12)

              // 任务卡片
              Column() {
                Row() {
                  Column() {
                    Text('自动答题任务').fontSize(14).fontWeight(FontWeight.Medium)
                    Text('手机/模拟考试不足24分自动补足试卷二')
                      .fontSize(12).fontColor(MUTED).margin({ top: 3 })
                  }
                  .alignItems(HorizontalAlign.Start)
                  .layoutWeight(1)

                  Button(this.running ? '停止' : '启动')
                    .width(72).height(34)
                    .backgroundColor(this.running ? '#E64340' : GREEN)
                    .fontColor(Color.White)
                    .fontSize(14)
                    .borderRadius(6)
                    .onClick(() => {
                      if (this.running) { this.stopTask(); } else { this.startTask(); }
                    })
                }
                .width('100%')
              }
              .width('100%')
              .padding(14)
              .margin({ top: 12, left: 12, right: 12 })
              .backgroundColor('#FFFFFF')
              .borderRadius(12)

              // 日志卡片
              Column() {
                Text('运行日志').fontSize(14).fontColor(MUTED).margin({ bottom: 8 })
                Column() {
                  ForEach(this.logs.slice(-30), (line: string) => {
                    Text(line).fontSize(11).fontColor('#D4D4D4').margin({ bottom: 2 })
                  })
                }
                .width('100%')
                .height(200)
                .padding(10)
                .backgroundColor('#1E1E1E')
                .borderRadius(10)
                .alignItems(HorizontalAlign.Start)
              }
              .width('100%')
              .padding(14)
              .margin({ top: 12, left: 12, right: 12, bottom: 12 })
              .backgroundColor('#FFFFFF')
              .borderRadius(12)
            }
            .width('100%')
          }
          .layoutWeight(1)
          .align(Alignment.Top)
          .backgroundColor(BG)
          .scrollBar(BarState.Off)
        }
        .width('100%').height('100%')
      }
    }
    .width('100%').height('100%')
  }
}