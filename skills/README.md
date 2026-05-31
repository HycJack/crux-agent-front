# 天气查询技能 (Weather Search Skill)

## 简介
通过命令行快速查询全球任意城市的天气信息，基于 [wttr.in](https://wttr.in) 服务。

## 使用方法

### 基本用法
```bash
# 查询杭州天气（默认）
./weather.sh

# 查询指定城市
./weather.sh Beijing
./weather.sh Shanghai
./weather.sh "New York"
./weather.sh Tokyo
```

### 参数说明
| 参数 | 说明 | 默认值 |
|------|------|--------|
| `$1` | 城市名（英文或拼音） | Hangzhou |

### 输出内容
1. **当前天气** — 天气状况、温度、湿度、风速、降水量
2. **三日预报** — 未来三天的天气图表
3. **文字摘要** — 一行式天气概况

## 示例输出
```
========================================
  🌤️  天气查询: Hangzhou
========================================

📍 当前天气:
   🌤️  Partly Cloudy  +24°C  49%  ←9km/h  0.0mm

📋 文字摘要:
   Hangzhou: Partly Cloudy, 温度:+24°C, 湿度:49%, 风速:←9km/h, 气压:1012hPa

数据来源: wttr.in | 查询时间: 2026-05-30 19:35:00
```

## 依赖
- `curl` — 用于发起 HTTP 请求
- `sed` — 用于文本处理

## 数据来源
[wttr.in](https://wttr.in) — 免费的命令行天气服务
