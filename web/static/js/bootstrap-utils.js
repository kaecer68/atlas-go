import { fmt, fmtPct, fmtFloat, fmtInt, pnlColor, pnlSign, convColor, escapeHtml, emptyState } from './shared/utils.js';
import { agentName as agentNameEsm, regimeLabel as regimeLabelEsm, AGENT_NAME_MAP, PAGE_TITLES } from './shared/constants.js';
import { renderEquityCurve, renderAgentScoreboard, renderRegimeContext, renderAllocationGuidance } from './components/sparkline.js';
Object.assign(window, { fmt, fmtPct, fmtFloat, fmtInt, pnlColor, pnlSign, convColor, escapeHtml, emptyState, agentNameEsm, regimeLabelEsm });
