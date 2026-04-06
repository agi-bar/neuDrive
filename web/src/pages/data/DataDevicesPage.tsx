import { useEffect, useState } from 'react'
import { api, type DeviceRecord } from '../../api'
import { formatDateTime, summarizeText } from './DataShared'

function sortDevices(entries: DeviceRecord[]) {
  return [...entries].sort((a, b) => {
    const aTime = new Date(a.updated_at || a.created_at || 0).getTime()
    const bTime = new Date(b.updated_at || b.created_at || 0).getTime()
    return bTime - aTime
  })
}

export default function DataDevicesPage() {
  const [devices, setDevices] = useState<DeviceRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const load = async () => {
      try {
        setDevices(sortDevices(await api.getDevices()))
      } catch (err: any) {
        setError(err.message || '加载设备失败')
      } finally {
        setLoading(false)
      }
    }

    load()
  }, [])

  if (loading) {
    return <div className="page-loading">加载中...</div>
  }

  return (
    <div className="page">
      <div className="page-header page-header-stack">
        <div>
          <h2>设备</h2>
          <p className="page-subtitle">这里显示当前 Hub 中登记的设备能力、协议和状态。</p>
        </div>
      </div>

      {error && <div className="alert alert-warn">{error}</div>}

      {devices.length === 0 ? (
        <div className="empty-state">
          <p>还没有设备</p>
          <p className="empty-hint">接入设备后，这里会展示它们的基本信息和状态。</p>
        </div>
      ) : (
        <div className="data-record-list">
          {devices.map((device) => (
            <div key={device.id || device.name} className="card data-record-item">
              <div className="data-record-head">
                <div className="data-record-title">{device.name}</div>
                <div className="data-inline-list">
                  {device.status && <span className="dashboard-inline-chip">{device.status}</span>}
                  {device.device_type && <span className="dashboard-inline-chip">{device.device_type}</span>}
                </div>
              </div>
              <div className="data-record-preview">
                {summarizeText(
                  [device.brand, device.protocol, device.endpoint].filter(Boolean).join(' / '),
                  220,
                )}
              </div>
              <div className="data-record-meta">{formatDateTime(device.updated_at || device.created_at)}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
