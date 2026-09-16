import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { LogicalProject } from '../api/types'

export default function ProjectsPage() {
  const [projects, setProjects] = useState<LogicalProject[]>([]); const [loading, setLoading] = useState(true); const [error, setError] = useState('')
  useEffect(() => { api.listProjects().then(result => setProjects(result.projects)).catch(reason => setError(reason instanceof Error ? reason.message : '项目加载失败')).finally(() => setLoading(false)) }, [])
  return <section><div className="page-title"><div><p className="eyebrow">项目身份与归属</p><h2>项目管理</h2><p className="muted">逻辑项目按 Git 远程仓库归并，克隆目录和工作树作为位置保留</p></div><button className="secondary" onClick={() => api.startProjectBackfill('dry_run', 1).catch(reason => setError(reason instanceof Error ? reason.message : '回填启动失败'))}>试运行历史归属</button></div>{error && <div className="state error-state">{error}</div>}{loading ? <div className="state">正在加载项目...</div> : projects.length === 0 ? <div className="state empty">暂无逻辑项目</div> : <div className="panel"><div className="table-wrap"><table><thead><tr><th>逻辑项目</th><th>版本控制</th><th>状态</th><th>位置</th><th>位置详情</th></tr></thead><tbody>{projects.map(project => <tr key={project.id}><td><strong>{project.display_name}</strong><small>{project.id}</small></td><td>{project.vcs}</td><td>{project.status === 'needs_review' ? '待复核' : project.status === 'archived' ? '已归档' : '活动'}</td><td>{project.location_count} 个位置</td><td>{project.locations.map(location => <div key={location.id}><strong>{location.display_name}</strong><small>{location.device_id} · {location.local_project_id}</small></div>)}</td></tr>)}</tbody></table></div></div>}</section>
}
