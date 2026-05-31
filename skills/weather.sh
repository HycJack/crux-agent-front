#!/bin/bash
# ============================================
# 天气查询脚本 (Weather Search Skill)
# 用法: ./weather.sh [城市名]
# 示例: ./weather.sh Hangzhou
#        ./weather.sh Beijing
#        ./weather.sh "New York"
# ============================================

CITY="${1:-Hangzhou}"
LANG_CODE="zh"

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  🌤️  天气查询: ${CITY}${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# 1. 当前天气
echo -e "${YELLOW}📍 当前天气:${NC}"
CURRENT=$(curl -s "https://wttr.in/${CITY}?lang=${LANG_CODE}&format=%c+%C+%t+%h+%w+%p" 2>/dev/null)
echo "   $CURRENT"
echo ""

# 2. 三日预报 (简洁模式)
echo -e "${YELLOW}📅 三日预报:${NC}"
curl -s "https://wttr.in/${CITY}?lang=${LANG_CODE}&format=v2&days=3" 2>/dev/null | sed 's/\x1b\[[0-9;]*m//g'
echo ""

# 3. 输出纯文本版本 (适合读取)
echo -e "${YELLOW}📋 文字摘要:${NC}"
SUMMARY=$(curl -s "https://wttr.in/${CITY}?lang=${LANG_CODE}&format=%l: %C, 温度:%t, 湿度:%h, 风速:%w, 气压:%p" 2>/dev/null)
echo "   $SUMMARY"
echo ""
echo -e "${CYAN}数据来源: wttr.in | 查询时间: $(date '+%Y-%m-%d %H:%M:%S')${NC}"
