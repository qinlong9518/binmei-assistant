#!/bin/sh

# 基于本地已缓存的 Gradle 8.14（可按需在 gradle-wrapper.properties 中切换版本）
APP_HOME=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)

if [ -x /root/.gradle/wrapper/dists/gradle-8.14-all/c2qonpi39x1mddn7hk5gh9iqj/gradle-8.14/bin/gradle ]; then
    exec /root/.gradle/wrapper/dists/gradle-8.14-all/c2qonpi39x1mddn7hk5gh9iqj/gradle-8.14/bin/gradle "$@"
fi

# 回退：按 wrapper properties 下载（需网络）
CLASSPATH=gradle/wrapper/gradle-wrapper.jar
exec java -classpath "$CLASSPATH" org.gradle.wrapper.GradleWrapperMain "$@"