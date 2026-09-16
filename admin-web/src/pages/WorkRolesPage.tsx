import { FormEvent, useEffect, useState } from 'react'
import { api } from '../api/client'
import type { RoleAssignment, Subject, WorkRole } from '../api/types'

const roles: WorkRole[] = ['研发', '测试', '产品']

export default function WorkRolesPage() {
  const [items, setItems] = useState<RoleAssignment[]>([])
  const [subjects, setSubjects] = useState<Subject[]>([])
  const [subjectID, setSubjectID] = useState('')
  const [role, setRole] = useState<WorkRole>('研发')
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let active = true
    Promise.all([api.listWorkRoles(), api.listSubjects()]).then(([assignments, availableSubjects]) => {
      if (!active) return
      setItems(assignments)
      setSubjects(availableSubjects)
      setSubjectID(current => availableSubjects.some(subject => subject.id === current) ? current : (availableSubjects[0]?.id || ''))
    }).catch(reason => {
      if (active) setError(reason instanceof Error ? reason.message : '角色数据加载失败')
    }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!subjectID) return
    setSaving(true); setMessage(''); setError('')
    try {
      const item = await api.assignWorkRole({ subject_id: subjectID, role })
      setItems(current => [item, ...current.filter(existing => existing.subject_id !== item.subject_id || (existing.project_id || '') !== (item.project_id || ''))])
      setMessage('工作角色已保存')
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const subjectName = new Map(subjects.map(subject => [subject.id, subject.name]))
  return <section>
    <div className="page-title"><div><p className="eyebrow">主体属性</p><h2>工作角色</h2><p className="muted">为采集主体标记研发、测试或产品角色</p></div></div>
    <form className="inline-form" onSubmit={submit}>
      <label className="role-subject">主体
        <select aria-label="主体" value={subjectID} onChange={event => setSubjectID(event.target.value)} required disabled={loading || !subjects.length}>
          {!subjects.length && <option value="">{loading ? '正在加载主体...' : '暂无可分配主体'}</option>}
          {subjects.map(subject => <option key={subject.id} value={subject.id}>{subject.name} · {subject.id}</option>)}
        </select>
      </label>
      <label>角色<select aria-label="角色" value={role} onChange={event => setRole(event.target.value as WorkRole)}>{roles.map(value => <option key={value}>{value}</option>)}</select></label>
      <button disabled={saving || !subjectID}>{saving ? '正在保存...' : '分配角色'}</button>
    </form>
    {message && <p className="notice">{message}</p>}
    {error && <p className="error">{error}</p>}
    {!loading && items.length === 0 ? <div className="state empty">暂无角色分配</div> : items.length > 0 && <div className="table-wrap"><table><thead><tr><th>主体</th><th>角色</th><th>项目</th></tr></thead><tbody>{items.map(item => <tr key={item.id || `${item.subject_id}-${item.role}`}><td><strong>{subjectName.get(item.subject_id) || item.subject_id}</strong><small>{item.subject_id}</small></td><td>{item.role}</td><td>{item.project_id || '租户默认'}</td></tr>)}</tbody></table></div>}
  </section>
}
