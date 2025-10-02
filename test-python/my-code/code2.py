class BankAccount:
    # 类变量
    interest_rate = 0.03

    def __init__(self, account_number, holder_name, balance=0):
        self.account_number = account_number
        self.holder_name = holder_name
        self.balance = balance

    def deposit(self, amount):
        if amount > 0:
            self.balance += amount
            return f"Deposited ${amount}. New balance: ${self.balance}"
        else:
            return "Deposit amount must be positive"

    def withdraw(self, amount):
        if 0 < amount <= self.balance:
            self.balance -= amount
            return f"Withdrew ${amount}. New balance: ${self.balance}"
        else:
            return "Invalid withdrawal amount or insufficient funds"

    def apply_interest(self):
        interest = self.balance * BankAccount.interest_rate
        self.balance += interest
        return f"Applied interest: ${interest:.2f}. New balance: ${self.balance:.2f}"

    def get_balance(self):
        return f"Current balance: ${self.balance}"


# 创建账户并执行操作
account1 = BankAccount("12345", "John Smith", 1000)
account2 = BankAccount("67890", "Jane Doe")

print(account1.get_balance())  # 输出: Current balance: $1000
print(account2.get_balance())  # 输出: Current balance: $0

print(account1.deposit(500))  # 输出: Deposited $500. New balance: $1500
print(account2.deposit(300))  # 输出: Deposited $300. New balance: $300

print(account1.withdraw(200))  # 输出: Withdrew $200. New balance: $1300
print(account2.withdraw(400))  # 输出: Invalid withdrawal amount or insufficient funds

print(account1.apply_interest())  # 输出: Applied interest: $39.00. New balance: $1339.00