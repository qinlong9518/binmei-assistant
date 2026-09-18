import UIAbility from '@ohos.app.ability.UIAbility';
import hilog from '@ohos.hilog';

export default class MainAbility extends UIAbility {
  onCreate(want, launchParam) {
    hilog.info(0x0000, 'BinmeiApp', '%{public}s', 'MainAbility onCreate');
  }

  onDestroy() {
    hilog.info(0x0000, 'BinmeiApp', '%{public}s', 'MainAbility onDestroy');
  }

  onWindowStageCreate(windowStage) {
    hilog.info(0x0000, 'BinmeiApp', '%{public}s', 'MainAbility onWindowStageCreate');
    windowStage.loadContent('pages/Index', (err, data) => {
      if (err.code) {
        hilog.error(0x0000, 'BinmeiApp', 'loadContent failed: %{public}s', JSON.stringify(err));
        return;
      }
      hilog.info(0x0000, 'BinmeiApp', '%{public}s', 'loadContent ok');
    });
  }

  onWindowStageDestroy() {}
  onForeground() {}
  onBackground() {}
}