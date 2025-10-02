# 学生成绩管理系统
students = [
    {
        "name": "Alice",
        "id": "A001",
        "scores": {"Math": 95, "Science": 88, "English": 92}
    },
    {
        "name": "Bob",
        "id": "B002",
        "scores": {"Math": 78, "Science": 85, "English": 80}
    },
    {
        "name": "Charlie",
        "id": "C003",
        "scores": {"Math": 90, "Science": 92, "English": 85}
    }
]

# 计算每个学生的平均分
for student in students:
    scores = student["scores"]
    average = sum(scores.values()) / len(scores)
    student["average"] = average
    print(f"{student['name']}'s average score: {average:.2f}")

# 找出平均分最高的学生
top_student = max(students, key=lambda s: s["average"])
print(f"\nTop student: {top_student['name']} with an average of {top_student['average']:.2f}")


