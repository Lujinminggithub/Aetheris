import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AuditLog } from '../api/types'
export default function AuditPage() { const [logs, setLogs] = useState<AuditLog[]>([]); useEffect(() => { api.listAuditLogs().then(setLogs) }, []); return <section><h2>审计日志</h2>{logs.length === 0 ? <p className="empty">暂无审计记录</p> : <table><thead><tr><th>操作者</th><th>操作</th><th>资源</th><th>结果</th><th>时间</th></tr></thead><tbody>{logs.map(log => <tr key={log.id}><td>{log.actor}</td><td>{log.action}</td><td>{log.resource}</td><td>{log.result}</td><td>{log.created_at}</td></tr>)}</tbody></table>}</section> }
