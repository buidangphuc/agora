import { RegisterForm } from "@/features/auth/RegisterForm";

export default function RegisterPage() {
  return (
    <section>
      <h1 className="mb-6 text-center text-lg font-semibold">Tạo tài khoản</h1>
      <RegisterForm />
    </section>
  );
}
