import { OIDC_SCOPES } from "../domain/scopes";
import { Checkbox } from "../../../shared/ui/Checkbox";

type ScopeSelectorProps = {
  readonly selectedScopes: readonly string[];
  readonly onToggle: (scopeId: string, checked: boolean) => void;
  readonly errorMessage?: string;
};

/** Multi-select for OAuth client allowed_scopes. */
export function ScopeSelector({ selectedScopes, onToggle, errorMessage }: ScopeSelectorProps) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="text-sm font-medium text-gray-700">Allowed scopes</legend>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {OIDC_SCOPES.map((scope) => (
          <Checkbox
            key={scope.id}
            label={scope.label}
            description={scope.description}
            checked={selectedScopes.includes(scope.id)}
            disabled={scope.required}
            onChange={(event) => onToggle(scope.id, event.currentTarget.checked)}
          />
        ))}
      </div>
      {errorMessage !== undefined ? (
        <p role="alert" className="text-xs text-red-600">
          {errorMessage}
        </p>
      ) : null}
    </fieldset>
  );
}
