import type { Settings } from '../../api/schema'
import { Timestamp } from '../status'

export function ChangesSection({ changes }: { changes: Settings['changes'] }) {
  return (
    <section className="table-surface">
      <table>
        <thead>
          <tr>
            <th>When</th>
            <th>Administrator</th>
            <th>Change</th>
            <th>Revision</th>
          </tr>
        </thead>
        <tbody>
          {changes.map((change) => (
            <tr key={change.revision}>
              <td>
                <Timestamp value={change.at} />
              </td>
              <td>{change.actor}</td>
              <td>{change.summary}</td>
              <td>{change.revision}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
