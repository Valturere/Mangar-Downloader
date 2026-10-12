import { Icon } from './Icon';
export function FolderField({
  id,
  label,
  path,
  disabled = false,
  onChoose,
}: {
  id: string;
  label: string;
  path: string;
  disabled?: boolean;
  onChoose?: () => void;
}) {
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      <button
        id={id}
        className="folder-picker"
        type="button"
        aria-label={onChoose ? 'Выбрать папку' : 'Выбрать папку по умолчанию'}
        aria-describedby={`${id}-path`}
        title={path || undefined}
        disabled={disabled}
        onClick={onChoose}
      >
        <Icon name="folder" />
        <span id={`${id}-path`} className="folder-path">
          {path ||
            (disabled && !onChoose ? 'Пока не настроена' : 'Выберите папку')}
        </span>
      </button>
    </div>
  );
}
