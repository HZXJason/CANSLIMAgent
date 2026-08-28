import unittest

import main


class _Response:
    content = "CPI 低于预期时，利率压力往往缓和，成长股估值通常更敏感。"


class _LLM:
    def invoke(self, messages):
        self.messages = messages
        return _Response()


class ResearchAssistantTest(unittest.TestCase):
    def test_answers_with_selected_model_and_bounded_prompt(self):
        llm = _LLM()
        request = main.ResearchAssistantRequest(
            question="CPI 对股市有哪些影响？",
            market="US",
            llm_provider="openai",
            model="gpt-test",
        )

        result = main.answer_research_question(request, llm_factory=lambda _request: llm)

        self.assertIn("利率压力", result["answer"])
        self.assertEqual(result["model"], "gpt-test")
        self.assertIn("CPI 对股市有哪些影响", llm.messages[-1].content)

    def test_rejects_empty_model_output(self):
        class EmptyLLM:
            def invoke(self, _messages):
                return type("Response", (), {"content": ""})()

        request = main.ResearchAssistantRequest(question="什么是 CPI？", market="US", llm_provider="openai", model="gpt-test")
        with self.assertRaisesRegex(RuntimeError, "empty"):
            main.answer_research_question(request, llm_factory=lambda _request: EmptyLLM())


if __name__ == "__main__":
    unittest.main()
