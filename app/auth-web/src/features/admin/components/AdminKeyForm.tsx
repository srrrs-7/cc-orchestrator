import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { Alert } from "../../../shared/ui/Alert";
import { Button } from "../../../shared/ui/Button";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { Input } from "../../../shared/ui/Input";
import { adminApiKeySchema, type AdminApiKeyFormValues } from "../api/schema";
import { useAdminAuth } from "../hooks/AdminAuthProvider";

type AdminKeyFormProps = {
  readonly onSuccess?: () => void;
};

/** Collects the static ADMIN_API_KEY used to call /admin/* endpoints. */
export function AdminKeyForm({ onSuccess }: AdminKeyFormProps) {
  const { setApiKey } = useAdminAuth();
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<AdminApiKeyFormValues>({
    resolver: zodResolver(adminApiKeySchema),
    defaultValues: { apiKey: "" },
  });

  const onSubmit = handleSubmit((values) => {
    setApiKey(values.apiKey);
    onSuccess?.();
  });

  return (
    <Card className="p-4 sm:p-5">
      <CardHeader
        title="Admin API key"
        description="Enter the ADMIN_API_KEY configured on the authorization server. The key is kept in this browser session only."
      />
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <label htmlFor="apiKey" className="text-sm font-medium text-gray-700">
            API key
          </label>
          <Input
            id="apiKey"
            type="password"
            autoComplete="off"
            placeholder="Bearer token for /admin/*"
            aria-invalid={errors.apiKey !== undefined}
            {...register("apiKey")}
          />
          {errors.apiKey ? (
            <p role="alert" className="text-xs text-red-600">
              {errors.apiKey.message}
            </p>
          ) : null}
        </div>
        <Button type="submit" disabled={isSubmitting} className="w-full sm:w-auto sm:self-start">
          Save key
        </Button>
      </form>
    </Card>
  );
}

type AdminKeyGateProps = {
  readonly children: React.ReactNode;
};

/** Renders children only after an admin API key has been stored. */
export function AdminKeyGate({ children }: AdminKeyGateProps) {
  const { isConfigured } = useAdminAuth();

  if (!isConfigured) {
    return (
      <div className="flex flex-col gap-4">
        <Alert variant="info">
          Configure your admin API key to provision users and OAuth clients.
        </Alert>
        <AdminKeyForm />
      </div>
    );
  }

  return <>{children}</>;
}
