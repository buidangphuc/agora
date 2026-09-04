import { LoginForm } from "@/features/auth/LoginForm";

export default function LoginPage() {
  return (
    <section>
      <h1 className="mb-6 text-center text-lg font-semibold">Đăng nhập</h1>
      <LoginForm />
    </section>
  );
}
