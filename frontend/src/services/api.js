import axios from 'axios'
import { globalConfig } from '@/lib/globalConfig'

const api = axios.create({
    baseURL: globalConfig.api.baseUrl
})

// v2 信封解包：仅当业务成功（error 为 null）时将 response.data.data 提升到 response.data；
// 业务错误（data=null, error={...}）保持原信封不动，调用方可读 error 字段提示。
api.interceptors.response.use((response) => {
    const body = response.data
    if (body && typeof body === 'object' && 'data' in body && 'meta' in body && body.error === null) {
        response.data = body.data
    }
    return response
})

export default api;

export const getStatus = () => api.get(globalConfig.api.endpoints.status)
export const getLatest = () => api.get(globalConfig.api.endpoints.latest)
export const getStats = () => api.get(globalConfig.api.endpoints.stats)
export const getBandwidth = () => api.get(globalConfig.api.endpoints.bandwidth)
export const scan = () => api.post(globalConfig.api.endpoints.scan)
export const getPowConfig = () => api.get(globalConfig.api.endpoints.powConfig)

// PoW 下载验证（替代极验）：创建挑战 → 浏览器求解 → 提交授权
export const createDownloadChallenge = (filePath) =>
    api.get(`${globalConfig.api.endpoints.downloadChallenge}?file_path=${encodeURIComponent(filePath)}`)

export const authorizeDownload = (challenge, solution) =>
    api.post(globalConfig.api.endpoints.downloadAuthorize, {
        challenge,
        solution
    })

export const prepareDownload = (filePath, returnUrl, source) =>
    api.post(globalConfig.api.endpoints.downloadPrepare, {
        file_path: filePath,
        ...(returnUrl && { return_url: returnUrl }),
        ...(source && { source: source })
    })

export const getDownloadLanding = (token) =>
    api.get(`${globalConfig.api.endpoints.downloadLanding}?token=${encodeURIComponent(token)}`)

// 文件完整性：SHA-256 校验值（PoW 验证页展示）
export const getFileIntegrity = (filePath) =>
    api.get(`${globalConfig.api.endpoints.fileIntegrity}?file_path=${encodeURIComponent(filePath)}`)

// 本节点身份：子节点名称与父节点地址（子节点标识/返回主节点入口）
export const getNodeInfo = () => api.get(globalConfig.api.endpoints.nodeInfo)

