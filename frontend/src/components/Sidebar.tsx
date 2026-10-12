import { Icon } from './Icon';
export type PageName = 'download' | 'settings';
export function Sidebar({
  page,
  onNavigate,
}: {
  page: PageName;
  onNavigate: (page: PageName) => void;
}) {
  return (
    <nav className="sidebar" aria-label="Основная навигация">
      <div className="nav-slot">
        <button
          type="button"
          className="nav-item"
          aria-label="Загрузка"
          aria-current={page === 'download' ? 'page' : undefined}
          aria-controls="page-content"
          onClick={() => onNavigate('download')}
        >
          <Icon name="download" />
        </button>
      </div>
      <div className="nav-slot">
        <button
          type="button"
          className="nav-item"
          aria-label="Настройки"
          aria-current={page === 'settings' ? 'page' : undefined}
          aria-controls="page-content"
          onClick={() => onNavigate('settings')}
        >
          <Icon name="settings" />
        </button>
      </div>
    </nav>
  );
}
