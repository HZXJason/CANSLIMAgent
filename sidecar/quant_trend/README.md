# Quant Trend Baseline

`quant_trend` 是 CANSLIMAgent 自有的确定性低频趋势基准。它不导入或调用任何 LLM，目的不是取代完整量化研究，而是为后续 `Q / TA / Q+TA` 配对消融提供冻结、可复算的对照组。

## v1 决策规则

`daily_tsm_sign_21_63_252.v1` 是受 Hurst、Ooi、Pedersen 经典趋势跟随方法启发的日频单股适配，同时计算以下独立证据：

- 21/63/252 个交易日时间序列动量：三个区间总收益的正负号等权投票。
- 12–2 总收益动量：`total_return_close[t-21] / total_return_close[t-251] - 1`。
- MA 趋势：价格相对 MA200、MA50 相对 MA200、MA50 的 20 日斜率。
- 52 周高点距离：价格收盘相对最近 252 个交易日最高收盘。
- 可选横截面动量百分位：同一交易日、同一版本股票池中的 12–2 动量平均排名。

v1 的主方向严格采用 21/63/252 日时间序列动量多数票；12–2 自身收益只有在同日、同版本股票池中转换成百分位后才成为相对强弱确认，MA 趋势和 52 周高点也只改变证据确认状态，不改变主方向。三个时间尺度通常不会平票，因此实际低频样本通常产生 `BUY` 或 `SELL`。

这不是原论文策略的逐项复现：原方法使用月末超额收益、多市场组合、等风险仓位和组合波动率目标。本模块服务于单标的方向消融，结果必须称为 Hurst-style adaptation，而不是 academic TSM replication。

输出中的 `signal_strength` 表示规则一致程度，不是概率。完成独立的 walk-forward 概率校准前，`probabilities` 始终为 `null`。

## 数据边界

默认源为 yfinance 的 Yahoo 日线：

- 原始 OHLC 用于 MA、52 周高点、ATR 和未来路径判分。
- `Adj Close` 作为含分红的总收益近似，用于全部时间序列/12–2 动量，以及探索性持有期收益；缺失时直接报错，不会退回 `Close` 改变研究语义。
- 请求明确使用 `auto_adjust=False`，避免混淆价格收益和总收益口径。
- 项目自有的单标的 canonical CSV 缓存会合并覆盖区间，避免每次 AI/实验重复下载同一份行情；不会在失败后静默混用其他供应商。
- 每份缓存和快照保存 `provider/config/data/universe` 哈希。

Yahoo 足以用于本地原型和单标的探索验证，但不包含无幸存者偏差的历史股票池。正式横截面论文实验必须替换 `PriceDataSource` 并提供 point-in-time universe，例如 CRSP、Sharadar 或 Norgate。不得把当前成分股回填历史后宣称为因果证据。

## 运行

```bash
cd sidecar

# 当前或历史节点的确定性快照
.venv/bin/python -m quant_trend predict MU --as-of 2025-12-31 --horizon 20

# 默认使用不重叠的 20 日节点，写入版本化 JSON Run Card
.venv/bin/python -m quant_trend evaluate MU \
  --start 2018-01-01 --end 2025-12-31 --horizon 20 --cost-bps 10
```

Sidecar 内部 API：

```text
POST /quant/trend/predict
POST /quant/trend/evaluate
```

可用 `QUANT_TREND_ARTIFACT_DIR` 修改默认的 `sidecar/data/quant-trend` 快照和评估目录，使用 `QUANT_TREND_PRICE_CACHE_DIR` 单独指定 canonical 行情缓存。

## 消融契约

`experiments.py` 固定三个实验臂：

- `quant_only`：只返回冻结量化快照，不进入 TradingAgents。
- `ta_blind`：移除量化快照和既有模型校准，独立运行 TradingAgents。
- `quant_informed`：与 `ta_blind` 使用相同上下文，唯一差异是加入冻结量化快照。

三臂必须共享相同 `node_id`、历史可见数据和唯一 outcome。普通 AI calibration 不可作为实验主存储，因为它的键和语义绑定 LLM 配置。

具体契约由 `experiments.py` 提供：

- `decision_node_id` 只包含标的、实际决策交易日、期限和下一交易日开盘入场规则，不包含策略版本或行情哈希；因此更换策略仍可连接到同一个未来结果。
- `snapshot_id` 固定量化策略、配置和当时可见数据，缓存读取会重新校验内容哈希及原请求字段。
- `prediction_id`、`outcome_id` 和 `manifest_id` 分别冻结预测、标签协议/未来路径、以及配对重复实验协议；原子存储目录彼此独立。
- `ta_blind` 的预测身份不依赖量化快照；`quant_informed` 才把冻结快照加入模型可见上下文。上下文会移除既有校准、交易日志、反思和 agent memory，并拒绝嵌套 future/outcome/other-arm 字段。

该文件目前只提供严格的实验边界，并未把消融 runner 接入普通 `/analyze`；因此现有 AI 分析行为不变。正式 runner 还必须固定 provider、model、采样参数、prompt/analyst/code 版本形成不含密钥的 `model_protocol_id`，对两个 TA 实验臂使用同一协议和重复次数。

## 评估口径

每个节点在决策后的下一交易日开盘进入，默认节点间隔等于预测周期，以避免未来窗口重叠。结果同时报告：

- 对称 ATR/最低 5% 的 first-hit 三重障碍标签；
- 期限终点趋势标签：20 日 ±1%，40 日按 `sqrt(2)` 缩放，中间为中性；
- 期限终点是否达到完整三重障碍的严格标签；
- 三分类准确率、balanced accuracy、方向覆盖率和方向准确率；
- 恒定 BUY/HOLD/SELL 基准；
- 量化策略及恒定 BUY/HOLD/SELL 在相同节点、相同成本口径下的探索性总回报和回撤。量化仓位使用三路投票的有符号比例（`±1/3` 或 `±1`），恒定基准使用满仓；极端空头亏损会清算至零，不允许负权益继续复利。

日线内同一天同时触及上下障碍时无法确定先后，节点标记 `AMBIGUOUS` 并从 first-hit 准确率中排除。

## 方法来源

- Moskowitz、Ooi、Pedersen 的时间序列动量研究：过去 12 个月超额收益用于预测下一月，多市场而非单股票分类（[论文 PDF](https://pages.stern.nyu.edu/~lpederse/papers/TimeSeriesMomentum.pdf)）。
- Hurst、Ooi、Pedersen 的长期趋势跟随研究：组合使用 1/3/12 个月信号，并包含月末采样与风险缩放（[论文 PDF](https://fairmodel.econ.yale.edu/ec439/hurst.pdf)）。
- 12–2 横截面形成期采用 Kenneth French 数据库的日频动量约定（[定义](https://mba.tuck.dartmouth.edu/pages/faculty/ken.french/Data_Library/f-f_developed_mom.html)）。
- 默认下载器及 `auto_adjust` 参数见 yfinance 官方文档（[API](https://ranaroussi.github.io/yfinance/reference/api/yfinance.download.html)）。
