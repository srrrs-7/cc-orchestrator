import { Link } from "@tanstack/react-router";
import { Alert } from "../../../shared/ui/Alert";
import { Button } from "../../../shared/ui/Button";
import { Card, CardHeader } from "../../../shared/ui/Card";
import { useDeleteUser } from "../hooks/useAdminMutations";
import { useUsersQuery } from "../hooks/useAdminQueries";

export function UserList() {
  const { data, isLoading, isError, error } = useUsersQuery();
  const deleteUser = useDeleteUser();

  const handleDelete = (userId: string, username: string) => {
    const confirmed = window.confirm(
      `Delete user "${username}" (${userId})? This cannot be undone.`,
    );
    if (!confirmed) {
      return;
    }
    deleteUser.mutate(userId);
  };

  return (
    <Card className="p-4 sm:p-5">
      <CardHeader
        title="Registered users"
        description="Resource owners who can authenticate and grant consent."
      />
      {deleteUser.isError ? <Alert className="mb-4">{deleteUser.error.message}</Alert> : null}
      {isLoading ? <p className="text-sm text-gray-500">Loading users…</p> : null}
      {isError ? <Alert className="mb-4">{error.message}</Alert> : null}
      {!isLoading && !isError && (data === undefined || data.length === 0) ? (
        <p className="text-sm text-gray-500">No users registered yet.</p>
      ) : null}
      {data !== undefined && data.length > 0 ? (
        <div className="overflow-x-auto">
          <table className="min-w-full text-left text-sm">
            <thead>
              <tr className="border-b border-border-subtle text-xs uppercase tracking-wide text-gray-500">
                <th className="px-2 py-2 font-medium">User ID</th>
                <th className="px-2 py-2 font-medium">Username</th>
                <th className="px-2 py-2 font-medium">Name</th>
                <th className="px-2 py-2 font-medium">Email</th>
                <th className="px-2 py-2 font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((user) => (
                <tr key={user.user_id} className="border-b border-border-subtle/70 last:border-0">
                  <td className="max-w-[12rem] truncate px-2 py-3 font-mono text-xs text-gray-800">
                    {user.user_id}
                  </td>
                  <td className="px-2 py-3 text-gray-900">{user.username}</td>
                  <td className="px-2 py-3 text-gray-900">{user.name}</td>
                  <td className="min-w-0 break-all px-2 py-3 text-gray-700">{user.email}</td>
                  <td className="px-2 py-3">
                    <div className="flex flex-wrap gap-2">
                      <Link
                        to="/users/$userId/edit"
                        params={{ userId: user.user_id }}
                        className="pointer-coarse:min-h-11 pointer-coarse:inline-flex pointer-coarse:items-center"
                      >
                        <Button type="button" variant="secondary">
                          Edit
                        </Button>
                      </Link>
                      <Button
                        type="button"
                        variant="secondary"
                        disabled={deleteUser.isPending}
                        onClick={() => handleDelete(user.user_id, user.username)}
                      >
                        Delete
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </Card>
  );
}
